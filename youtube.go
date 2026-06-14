package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func isYouTubeURL(s string) bool {
	return strings.Contains(s, "youtube.com") || strings.Contains(s, "youtu.be")
}

func getYouTubeTitle(videoURL string) (string, error) {
	cmd := exec.Command("yt-dlp",
		"--print", "%(title)s",
		"--no-playlist",
		videoURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func getStreamURL(videoURL string) (string, error) {
	cmd := exec.Command("yt-dlp",
		"-f", "bestaudio",
		"--get-url",
		"--no-playlist",
		videoURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp --get-url: %w", err)
	}
	// yt-dlp may return multiple URLs for split streams; take the first.
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return strings.TrimSpace(line), nil
}
