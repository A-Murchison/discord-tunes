package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	spotify "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

var spotifyClient *spotify.Client

// initSpotify authenticates with Spotify using Client Credentials (no user login).
// It logs a warning and leaves spotifyClient nil if credentials are absent.
func initSpotify() error {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_ID and/or SPOTIFY_CLIENT_SECRET not set - Spotify URLs will not work")
	}

	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	httpClient := cfg.Client(context.Background())
	spotifyClient = spotify.New(httpClient)
	return nil
}

// isSpotifyURL reports whether s looks like a Spotify open.spotify.com URL.
func isSpotifyURL(s string) bool {
	return strings.Contains(s, "open.spotify.com")
}

// resolveSpotifyURL parses a Spotify URL and returns the tracks to queue.
// Track.URL is set to a ytsearch1: pseudo-URL that yt-dlp resolves at playback time.
func resolveSpotifyURL(rawURL, voiceChannelID string) ([]Track, error) {
	if spotifyClient == nil {
		return nil, fmt.Errorf("Spotify is not configured (missing credentials)")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Strip query params (e.g. ?si=...) - the path is sufficient.
	// Path may be /track/<id>, /playlist/<id>, or locale-prefixed like /intl-es/playlist/<id>.
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")

	knownTypes := map[string]bool{"track": true, "playlist": true}
	kindIdx := -1
	for i, p := range parts {
		if knownTypes[p] {
			kindIdx = i
			break
		}
	}
	if kindIdx == -1 || kindIdx+1 >= len(parts) {
		return nil, fmt.Errorf("could not parse Spotify URL path: %s", u.Path)
	}

	kind := parts[kindIdx]
	id := spotify.ID(parts[kindIdx+1])

	switch kind {
	case "track":
		return resolveSpotifyTrack(id, voiceChannelID)
	case "playlist":
		return resolveSpotifyPlaylist(id, voiceChannelID)
	default:
		return nil, fmt.Errorf("unsupported Spotify URL type %q (only track and playlist are supported)", kind)
	}
}

// spotifyAPIError converts a zmb3/spotify API error into a user-friendly message.
func spotifyAPIError(action string, err error) error {
	var se *spotify.Error
	if errors.As(err, &se) {
		switch se.Status {
		case 403:
			return fmt.Errorf("Spotify: %s is private or restricted - set it to **Public** in Spotify and try again", action)
		case 404:
			return fmt.Errorf("Spotify: %s not found - check the URL is correct", action)
		case 401:
			return fmt.Errorf("Spotify: authentication failed - check SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET")
		}
	}
	return fmt.Errorf("Spotify: could not fetch %s: %w", action, err)
}

func resolveSpotifyTrack(id spotify.ID, voiceChannelID string) ([]Track, error) {
	ctx := context.Background()
	t, err := spotifyClient.GetTrack(ctx, id)
	if err != nil {
		return nil, spotifyAPIError("track", err)
	}
	query := spotifySearchQuery(t.Artists, t.Name)
	return []Track{{
		URL:            "ytsearch1:" + query,
		Title:          query,
		VoiceChannelID: voiceChannelID,
	}}, nil
}

func resolveSpotifyPlaylist(id spotify.ID, voiceChannelID string) ([]Track, error) {
	ctx := context.Background()
	var tracks []Track
	offset := 0
	const limit = 100

	for {
		page, err := spotifyClient.GetPlaylistItems(ctx, id,
			spotify.Limit(limit),
			spotify.Offset(offset),
		)
		if err != nil {
			return nil, spotifyAPIError("playlist", err)
		}

		for _, item := range page.Items {
			// item.Track.Track is nil for local/unavailable tracks
			if item.Track.Track == nil {
				continue
			}
			t := item.Track.Track
			query := spotifySearchQuery(t.Artists, t.Name)
			tracks = append(tracks, Track{
				URL:            "ytsearch1:" + query,
				Title:          query,
				VoiceChannelID: voiceChannelID,
			})
		}

		offset += len(page.Items)
		if offset >= int(page.Total) {
			break
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("Spotify playlist appears to be empty or contains only unavailable tracks")
	}
	return tracks, nil
}

// spotifySearchQuery returns "FirstArtist - Track Name" for use as a YouTube search term.
func spotifySearchQuery(artists []spotify.SimpleArtist, name string) string {
	artist := "Unknown Artist"
	if len(artists) > 0 {
		artist = artists[0].Name
	}
	return artist + " - " + name
}
