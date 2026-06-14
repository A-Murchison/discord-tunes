package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
)

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

	if strings.HasPrefix(m.Content, "!diag") {
		go runDiag(s, m.ChannelID)
	}

	if strings.HasPrefix(m.Content, "!test") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				player := getPlayer(g.ID)
				player.mu.Lock()
				vcNil := player.vc == nil
				player.mu.Unlock()
				if vcNil {
					fmt.Printf("[test] Joining voice channel %s\n", vs.ChannelID)
					vc, err := s.ChannelVoiceJoin(context.Background(), g.ID, vs.ChannelID, false, false)
					if err != nil {
						fmt.Println("[test] Error joining voice channel:", err)
						s.ChannelMessageSend(m.ChannelID, "Could not join your voice channel: "+err.Error())
						return
					}
					fmt.Println("[test] Joined voice channel")
					player.mu.Lock()
					if player.vc == nil {
						player.vc = vc
					} else {
						go vc.Disconnect(context.Background())
					}
					player.mu.Unlock()
				}
				s.ChannelMessageSend(m.ChannelID, "Playing test tone (440 Hz sine wave, 5 seconds)...")
				go player.playTestTone()
				return
			}
		}
		s.ChannelMessageSend(m.ChannelID, "You must be in a voice channel to use `!test`.")
	}

	if strings.HasPrefix(m.Content, "!play") || strings.HasPrefix(m.Content, "!queue") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				// Rejoin parts[1:] so markdown links with spaces still work, then
				// strip whatever URL formatting Discord may have applied.
				parts := strings.Fields(m.Content)
				if len(parts) < 2 {
					s.ChannelMessageSend(m.ChannelID, "Please provide a YouTube or Spotify URL: `!play <URL>`")
					return
				}
				inputURL := extractURL(strings.Join(parts[1:], ""))

				if isSpotifyURL(inputURL) {
					tracks, err := resolveSpotifyURL(inputURL, vs.ChannelID)
					if err != nil {
						s.ChannelMessageSend(m.ChannelID, "Spotify error: "+err.Error())
						return
					}
					player := getPlayer(g.ID)
					for _, t := range tracks {
						player.Enqueue(s, g.ID, t)
					}
					if len(tracks) == 1 {
						s.ChannelMessageSend(m.ChannelID, "Added to queue: **"+tracks[0].Title+"**")
					} else {
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Queued %d track(s) from Spotify", len(tracks)))
					}
					return
				}

				if !isYouTubeURL(inputURL) {
					s.ChannelMessageSend(m.ChannelID, "Please provide a YouTube or Spotify URL: `!play <URL>`")
					return
				}
				title, err := getYouTubeTitle(inputURL)
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, "Could not load that video: "+err.Error())
					return
				}
				track := Track{URL: inputURL, Title: title, VoiceChannelID: vs.ChannelID}
				getPlayer(g.ID).Enqueue(s, g.ID, track)
				s.ChannelMessageSend(m.ChannelID, "Added to queue: **"+title+"**")
				return
			}
		}
	}

	if strings.HasPrefix(m.Content, "!playlist") {
		g, shouldReturn := getGuild(s, m)
		if shouldReturn {
			return
		}
		queue := getPlayer(g.ID).Queue()
		if len(queue) == 0 {
			s.ChannelMessageSend(m.ChannelID, "The queue is empty.")
		} else {
			var sb strings.Builder
			sb.WriteString("**Queue:**\n")
			const maxShow = 10
			show := len(queue)
			if show > maxShow {
				show = maxShow
			}
			for i := 0; i < show; i++ {
				fmt.Fprintf(&sb, "%d. **%s**\n", i+1, queue[i].Title)
			}
			if len(queue) > maxShow {
				fmt.Fprintf(&sb, "*(and %d more)*", len(queue)-maxShow)
			}
			s.ChannelMessageSend(m.ChannelID, sb.String())
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

func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	fmt.Printf("Joined guild: %v\n", event.Name)
}

func getGuild(s *discordgo.Session, m *discordgo.MessageCreate) (*discordgo.Guild, bool) {
	c, err := s.State.Channel(m.ChannelID)
	if err != nil {
		fmt.Println("Error fetching channel:", err)
		return nil, true
	}
	g, err := s.State.Guild(c.GuildID)
	if err != nil {
		return nil, true
	}
	return g, false
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

// extractURL unwraps Discord URL formatting - markdown links ([text](url)) and
// angle-bracket suppressed URLs (<url>) - returning the bare URL.
func extractURL(s string) string {
	// Markdown hyperlink: [anything](url)
	if strings.HasPrefix(s, "[") {
		start := strings.LastIndex(s, "(")
		end := strings.LastIndex(s, ")")
		if start != -1 && end > start {
			return s[start+1 : end]
		}
	}
	// Angle-bracket suppressed: <url>
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return s[1 : len(s)-1]
	}
	return s
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
	select {
	case p.skip <- struct{}{}:
	default:
	}
}

func (p *GuildPlayer) ClearQueue() {
	p.mu.Lock()
	p.queue = nil
	p.mu.Unlock()
}

// Queue returns a snapshot of the current queue, safe for concurrent use.
func (p *GuildPlayer) Queue() []Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return nil
	}
	result := make([]Track, len(p.queue))
	copy(result, p.queue)
	return result
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

		fmt.Printf("[player] Playing: %s\n", track.Title)

		if p.vc == nil {
			fmt.Printf("[player] Joining voice channel %s\n", track.VoiceChannelID)
			vc, err := s.ChannelVoiceJoin(context.Background(), guildID, track.VoiceChannelID, false, false)
			if err != nil {
				fmt.Println("[player] Error joining voice channel:", err)
				continue
			}
			fmt.Println("[player] Joined voice channel")
			p.vc = vc
		}

		p.playTrack(track)
	}
}

func (p *GuildPlayer) playTrack(t Track) {
	fmt.Printf("[track] Getting direct stream URL for: %s\n", t.Title)
	streamURL, err := getStreamURL(t.URL)
	if err != nil {
		fmt.Println("[track] Error getting stream URL:", err)
		return
	}
	fmt.Printf("[track] Got stream URL (%d chars)\n", len(streamURL))

	// Pass reconnect flags so FFmpeg can recover from CDN URL expiry mid-stream.
	ffmpegArgs := []string{
		"-reconnect", "1",
		"-reconnect_at_eof", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"-i", streamURL,
		"-map", "0:a",
	}
	p.streamOpus("[track]", ffmpegArgs)
}

// playTestTone streams a 5s 440 Hz sine wave via FFmpeg's lavfi input.
// No yt-dlp involved - useful for isolating the voice pipeline from YouTube.
func (p *GuildPlayer) playTestTone() {
	fmt.Println("[test] Streaming 5-second 440 Hz test tone")
	p.streamOpus("[test]", []string{
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=5",
	})
}

// streamOpus encodes audio through FFmpeg → OGG/Opus and pushes raw frames to
// Discord. It intentionally avoids jonas747/dca which hardcodes "-vbr on", a
// flag removed in FFmpeg 7+ (correct form is "-vbr 1").
func (p *GuildPlayer) streamOpus(label string, ffmpegInputArgs []string) {
	args := make([]string, 0, len(ffmpegInputArgs)+14)
	args = append(args, ffmpegInputArgs...)
	args = append(args,
		"-c:a", "libopus",
		"-b:a", "96k",
		"-vbr", "1",
		"-frame_duration", "20",
		"-ar", "48000",
		"-ac", "2",
		"-application", "audio",
		"-f", "ogg",
		"pipe:1",
	)

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("%s Error creating stdout pipe: %v\n", label, err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("%s Error starting FFmpeg: %v\n", label, err)
		return
	}
	fmt.Printf("%s FFmpeg started (pid %d)\n", label, cmd.Process.Pid)

	// Decode OGG packets in a goroutine so FFmpeg's pipe never fills up.
	// Buffer 100 frames (~2 seconds) to smooth over decode jitter.
	frameCh := make(chan []byte, 100)
	go func() {
		defer close(frameCh)
		dec := ogg.NewPacketDecoder(ogg.NewDecoder(stdout))
		headerCount := 0
		for {
			packet, _, err := dec.Decode()
			if err != nil {
				if err != io.EOF {
					fmt.Printf("%s OGG decode error: %v\n", label, err)
				}
				return
			}
			// Skip the first two OGG packets: OpusHead and OpusTags headers, not audio.
			if headerCount < 2 {
				headerCount++
				continue
			}
			frameCh <- packet
		}
	}()

	// Wait for DAVE E2E encryption to be ready before sending audio.
	// Frames sent beforehand are silently dropped by Discord clients.
	// Returns immediately when DAVE is not active (dave == nil).
	daveCtx, daveCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer daveCancel()
	if err := p.vc.WaitForDAVEReady(daveCtx); err != nil {
		fmt.Printf("%s Warning: DAVE not ready after 3s: %v - audio may be silent\n", label, err)
	} else {
		fmt.Printf("%s DAVE ready, streaming audio\n", label)
	}

	p.vc.Speaking(true)
	framesSent := 0
	streamStart := time.Now()
	skipped := false

loop:
	for {
		select {
		case <-p.skip:
			fmt.Printf("%s Skipped after %d frames\n", label, framesSent)
			cmd.Process.Kill()
			skipped = true
			break loop

		case frame, ok := <-frameCh:
			if !ok {
				// Reader goroutine closed the channel - stream ended normally.
				break loop
			}
			// Push directly into OpusSend. The voice connection's opusSender goroutine
			// has its own 20ms ticker for UDP pacing. A second ticker here would double
			// the frame interval (40ms) and cause Discord to receive audio at half rate.
			select {
			case p.vc.OpusSend <- frame:
				framesSent++
			case <-p.skip:
				fmt.Printf("%s Skipped after %d frames\n", label, framesSent)
				cmd.Process.Kill()
				skipped = true
				break loop
			}
		}
	}
	p.vc.Speaking(false)

	// Drain frameCh so the reader goroutine is never blocked on a send.
	for range frameCh {
	}

	if err := cmd.Wait(); err != nil && !strings.Contains(err.Error(), "killed") {
		fmt.Printf("%s FFmpeg exited with error: %v\n", label, err)
	}

	// Only log the last few FFmpeg stderr lines - FFmpeg writes a progress line per frame.
	if output := strings.TrimSpace(stderr.String()); output != "" {
		lines := strings.Split(output, "\n")
		startIdx := len(lines) - 5
		if startIdx < 0 {
			startIdx = 0
		}
		fmt.Printf("%s FFmpeg stderr (last %d lines):\n%s\n",
			label, len(lines[startIdx:]), strings.Join(lines[startIdx:], "\n"))
	}

	if !skipped {
		fmt.Printf("%s Finished: %d frames sent in %v\n", label, framesSent, time.Since(streamStart))
	}
}

func runDiag(s *discordgo.Session, channelID string) {
	var msg strings.Builder
	msg.WriteString("**discord-tunes diagnostics**\n")

	vOut, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		msg.WriteString("`ffmpeg`: **NOT FOUND** - install FFmpeg and ensure it is in PATH\n")
	} else {
		firstLine := strings.SplitN(string(vOut), "\n", 2)[0]
		msg.WriteString(fmt.Sprintf("`ffmpeg`: %s\n", firstLine))
	}

	encsOut, _ := exec.Command("ffmpeg", "-encoders").CombinedOutput()
	encsStr := string(encsOut)
	switch {
	case strings.Contains(encsStr, "libopus"):
		msg.WriteString("`libopus` encoder: **FOUND** ✓\n")
	case strings.Contains(encsStr, " opus"):
		msg.WriteString("`libopus` encoder: **NOT FOUND** - only the native `opus` encoder is present.\n")
		msg.WriteString("> Download a **full** FFmpeg build: <https://www.gyan.dev/ffmpeg/builds/>\n")
	default:
		msg.WriteString("`libopus` encoder: **NOT FOUND** - no Opus encoder detected at all.\n")
		msg.WriteString("> Download a **full** FFmpeg build: <https://www.gyan.dev/ffmpeg/builds/>\n")
	}

	ytOut, err := exec.Command("yt-dlp", "--version").Output()
	if err != nil {
		msg.WriteString("`yt-dlp`: **NOT FOUND** - install yt-dlp and ensure it is in PATH\n")
	} else {
		msg.WriteString(fmt.Sprintf("`yt-dlp`: %s\n", strings.TrimSpace(string(ytOut))))
	}

	var opusStderr bytes.Buffer
	opusTest := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=0.1",
		"-c:a", "libopus",
		"-b:a", "96k",
		"-vbr", "1",
		"-frame_duration", "20",
		"-ar", "48000",
		"-ac", "2",
		"-f", "ogg",
		"-",
	)
	opusTest.Stderr = &opusStderr
	opusOut, opusErr := opusTest.Output()
	if opusErr != nil {
		msg.WriteString(fmt.Sprintf("**Opus encode test: FAILED** (exit: `%v`)\n", opusErr))
		lines := strings.Split(strings.TrimSpace(opusStderr.String()), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				msg.WriteString(fmt.Sprintf("> Last FFmpeg line: `%s`\n", lines[i]))
				break
			}
		}
		for _, l := range lines {
			if strings.Contains(l, "Error") || strings.Contains(l, "Invalid") || strings.Contains(l, "No such") {
				msg.WriteString(fmt.Sprintf("> FFmpeg error: `%s`\n", l))
				break
			}
		}
	} else {
		msg.WriteString(fmt.Sprintf("Opus encode test: **PASSED** ✓ (%d bytes)\n", len(opusOut)))
	}

	s.ChannelMessageSend(channelID, msg.String())
}

func stop(s *discordgo.Session, guildID string) error {
	return nil
}
