// Init main package
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	youtube "github.com/kkdai/youtube/v2"
)

var token string

var (
	guildsMu sync.Mutex
	guilds   = make(map[string]*GuildPlayer)
)

type Track struct {
	URL            string
	Title          string
	VoiceChannelID string
}

type GuildPlayer struct {
	mu      sync.Mutex
	queue   []Track
	vc      *discordgo.VoiceConnection
	skip    chan struct{}
	playing bool
}

func init() {
	flag.StringVar(&token, "t", "", "Token for bot")
	flag.Parse()
}

// Main function
func main() {

	if token == "" {
		fmt.Println("No token provided. Please set the token variable.")
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println(err)
	}

	// Register ready as a call back for the ready events
	discord.AddHandler(ready)

	// Register ready as a call back for the message create events
	discord.AddHandler(messageCreate)

	// Register ready as a call back for the guild create events
	discord.AddHandler(guildCreate)

	// We need information about guilds (which includes their channels) messages and voice states
	discord.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentsMessageContent

	// Open the websocket
	err = discord.Open()
	if err != nil {
		fmt.Println("Error opening Discord session:", err)
		return
	}
	defer discord.Close()

	fmt.Println("Bot is now running. Press CTRL-C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	fmt.Printf("Logged in as: %v#%v\n", s.State.User.Username, s.State.User.Discriminator)

	s.UpdateGameStatus(0, "Playing some tasty tunes")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, "!ping") {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}

	if strings.HasPrefix(m.Content, "!play") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				parts := strings.Fields(m.Content)
				if len(parts) < 2 || (!strings.Contains(parts[1], "youtube.com") && !strings.Contains(parts[1], "youtu.be")) {
					s.ChannelMessageSend(m.ChannelID, "Please provide a valid YouTube URL: `!play <YouTube URL>`")
					return
				}
				streamURL, title, err := getYouTubeStreamURL(parts[1])
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, "Could not load that video: "+err.Error())
					return
				}
				track := Track{URL: streamURL, Title: title, VoiceChannelID: vs.ChannelID}
				getPlayer(g.ID).Enqueue(s, g.ID, track)
				s.ChannelMessageSend(m.ChannelID, "Added to queue: **"+title+"**")
				return
			}
		}
	}

	if strings.HasPrefix(m.Content, "!queue") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				parts := strings.Fields(m.Content)
				if len(parts) < 2 || (!strings.Contains(parts[1], "youtube.com") && !strings.Contains(parts[1], "youtu.be")) {
					s.ChannelMessageSend(m.ChannelID, "Please provide a valid YouTube URL: `!queue <YouTube URL>`")
					return
				}
				streamURL, title, err := getYouTubeStreamURL(parts[1])
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, "Could not load that video: "+err.Error())
					return
				}
				track := Track{URL: streamURL, Title: title, VoiceChannelID: vs.ChannelID}
				getPlayer(g.ID).Enqueue(s, g.ID, track)
				s.ChannelMessageSend(m.ChannelID, "Added to queue: **"+title+"**")
				return
			}
		}
	}

	if strings.HasPrefix(m.Content, "!skip") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}

		getPlayer(g.ID).Skip()
	}

	if strings.HasPrefix(m.Content, "!stop") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}

		getPlayer(g.ID).ClearQueue()
		getPlayer(g.ID).Skip()
	}

	if strings.HasPrefix(m.Content, "!clear") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}

		getPlayer(g.ID).ClearQueue()
	}
}

func getGuild(s *discordgo.Session, m *discordgo.MessageCreate) (*discordgo.Guild, bool) {
	c, err := s.State.Channel(m.ChannelID)
	if err != nil {
		fmt.Println("Error fetching channel:", err)
		return nil, true
	}

	// Find the guild for that channel.
	g, err := s.State.Guild(c.GuildID)
	if err != nil {
		// Could not find guild.
		return nil, true
	}
	return g, false
}

func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	fmt.Printf("Joined guild: %v\n", event.Name)
}

// getPlayer retrieves the GuildPlayer for a given guild ID, creating one if it doesn't exist.
func getPlayer(guildID string) *GuildPlayer {
	guildsMu.Lock()
	defer guildsMu.Unlock()
	if p, ok := guilds[guildID]; ok {
		return p
	}
	p := &GuildPlayer{
		skip: make(chan struct{}, 1),
	}
	guilds[guildID] = p
	return p
}

func getYouTubeStreamURL(videoURL string) (string, string, error) {
	client := youtube.Client{}
	video, err := client.GetVideo(videoURL)
	if err != nil {
		return "", "", err
	}
	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		return "", "", fmt.Errorf("no audio formats found for this video")
	}
	// Prefer audio/webm — WebM supports stdin piping; audio/mp4 requires seeking
	for i := range formats {
		if strings.HasPrefix(formats[i].MimeType, "audio/webm") {
			url, err := client.GetStreamURL(video, &formats[i])
			if err != nil {
				return "", "", err
			}
			return url, video.Title, nil
		}
	}
	url, err := client.GetStreamURL(video, &formats[0])
	if err != nil {
		return "", "", err
	}
	return url, video.Title, nil
}

func (p *GuildPlayer) Enqueue(s *discordgo.Session, guildID string, t Track) {
	p.mu.Lock()
	p.queue = append(p.queue, t)
	start := !p.playing
	p.mu.Unlock()
	if start {
		go p.playLoop(s, guildID)
	}
}

func (p *GuildPlayer) Skip() {
	p.skip <- struct{}{}
}

func (p *GuildPlayer) ClearQueue() {
	p.mu.Lock()
	p.queue = nil
	p.mu.Unlock()
}

func (p *GuildPlayer) playLoop(s *discordgo.Session, guildID string) {
	p.mu.Lock()
	p.playing = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.playing = false
		p.mu.Unlock()
	}()

	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.mu.Unlock()
			if p.vc != nil {
				p.vc.Disconnect(context.Background())
				p.vc = nil
			}
			return
		}
		track := p.queue[0]
		p.queue = p.queue[1:]
		p.mu.Unlock()

		if p.vc == nil {
			vc, err := s.ChannelVoiceJoin(context.Background(), guildID, track.VoiceChannelID, false, true)
			if err != nil {
				fmt.Println("Error joining voice channel:", err)
				continue
			}
			p.vc = vc
		}

		p.playTrack(track)
	}
}

func (p *GuildPlayer) playTrack(t Track) {
	req, err := http.NewRequest("GET", t.URL, nil)
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; discord-tunes)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error fetching audio stream:", err)
		return
	}
	defer resp.Body.Close()

	opts := *dca.StdEncodeOptions
	opts.RawOutput = true
	opts.Bitrate = 96

	encSession, err := dca.EncodeMem(resp.Body, &opts)
	if err != nil {
		fmt.Println("Error encoding audio:", err)
		return
	}
	defer encSession.Cleanup()

	done := make(chan error, 1)
	dca.NewStream(encSession, p.vc, done)

	select {
	case <-p.skip:
	case err = <-done:
		if err != nil && err != io.EOF {
			fmt.Println("Error streaming audio:", err)
		}
	}

	if encErr := encSession.Error(); encErr != nil {
		fmt.Println("FFmpeg encode error:", encErr)
	}
}

func queueSpotifySong(s1 *discordgo.Session, s2, s3 string) error {
	// This is where you would implement the logic to queue a Spotify song in the voice channel.
	// You would need to use a library that can handle audio streaming to Discord, such as "github.com/jonas747/dca".
	// You would also need to handle authentication with Spotify's API to fetch the song data.
	return nil
}

func playSpotifySong(s *discordgo.Session, guildID, channelID string) error {
	// This is where you would implement the logic to play a Spotify song in the voice channel.
	// You would need to use a library that can handle audio streaming to Discord, such as "github.com/jonas747/dca".
	// You would also need to handle authentication with Spotify's API to fetch the song data.
	return nil
}

func stop(s *discordgo.Session, guildID string) error {
	// This is where you would implement the logic to stop the currently playing song in the voice channel.
	// You would need to manage the audio stream and ensure that it can be stopped when requested.
	return nil
}
