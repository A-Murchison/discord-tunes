// Init main package
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
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
				if player.vc == nil {
					fmt.Printf("[test] Joining voice channel %s\n", vs.ChannelID)
					vc, err := s.ChannelVoiceJoin(context.Background(), g.ID, vs.ChannelID, false, false)
					if err != nil {
						fmt.Println("[test] Error joining voice channel:", err)
						s.ChannelMessageSend(m.ChannelID, "Could not join your voice channel: "+err.Error())
						return
					}
					fmt.Println("[test] Joined voice channel")
					player.mu.Lock()
					player.vc = vc
					player.mu.Unlock()
				}
				s.ChannelMessageSend(m.ChannelID, "Playing test tone (440 Hz sine wave, 5 seconds)...")
				go player.playTestTone()
				return
			}
		}
		s.ChannelMessageSend(m.ChannelID, "You must be in a voice channel to use `!test`.")
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
				title, err := getYouTubeTitle(parts[1])
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, "Could not load that video: "+err.Error())
					return
				}
				track := Track{URL: parts[1], Title: title, VoiceChannelID: vs.ChannelID}
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
				title, err := getYouTubeTitle(parts[1])
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, "Could not load that video: "+err.Error())
					return
				}
				track := Track{URL: parts[1], Title: title, VoiceChannelID: vs.ChannelID}
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
	// yt-dlp may return multiple URLs for split streams; take the first
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return strings.TrimSpace(line), nil
}

func runDiag(s *discordgo.Session, channelID string) {
	var msg strings.Builder
	msg.WriteString("**discord-tunes diagnostics**\n")

	// FFmpeg version
	vOut, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		msg.WriteString("`ffmpeg`: **NOT FOUND** — install FFmpeg and ensure it is in PATH\n")
	} else {
		firstLine := strings.SplitN(string(vOut), "\n", 2)[0]
		msg.WriteString(fmt.Sprintf("`ffmpeg`: %s\n", firstLine))
	}

	// libopus encoder
	encsOut, _ := exec.Command("ffmpeg", "-encoders").CombinedOutput()
	encsStr := string(encsOut)
	switch {
	case strings.Contains(encsStr, "libopus"):
		msg.WriteString("`libopus` encoder: **FOUND** ✓\n")
	case strings.Contains(encsStr, " opus"):
		msg.WriteString("`libopus` encoder: **NOT FOUND** — only the native `opus` encoder is present.\n")
		msg.WriteString("> Download a **full** FFmpeg build: <https://www.gyan.dev/ffmpeg/builds/>\n")
	default:
		msg.WriteString("`libopus` encoder: **NOT FOUND** — no Opus encoder detected at all.\n")
		msg.WriteString("> Download a **full** FFmpeg build: <https://www.gyan.dev/ffmpeg/builds/>\n")
	}

	// yt-dlp version
	ytOut, err := exec.Command("yt-dlp", "--version").Output()
	if err != nil {
		msg.WriteString("`yt-dlp`: **NOT FOUND** — install yt-dlp and ensure it is in PATH\n")
	} else {
		msg.WriteString(fmt.Sprintf("`yt-dlp`: %s\n", strings.TrimSpace(string(ytOut))))
	}

	// Full end-to-end encode test using the exact same FFmpeg flags as the bot.
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

// playTestTone streams a 5-second 440 Hz sine wave directly from FFmpeg's lavfi
// input — no yt-dlp, no temp files. Isolates the voice pipeline from YouTube.
func (p *GuildPlayer) playTestTone() {
	fmt.Println("[test] Streaming 5-second 440 Hz test tone")
	p.streamOpus("[test]", []string{
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=5",
	})
}

// streamOpus runs FFmpeg with the provided input arguments, encodes to Opus
// inside an OGG container, parses the OGG pages, and sends raw Opus frames to
// Discord's OpusSend channel at the required 20 ms pacing.
//
// This replaces the jonas747/dca library which hardcodes "-vbr on", a flag
// that was removed in FFmpeg 7+ (it must now be "-vbr 1").
func (p *GuildPlayer) streamOpus(label string, ffmpegInputArgs []string) {
	// Append the shared output encoding flags to whatever input args were given.
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
	// Buffer 100 frames (~2 seconds) to smooth over any decode jitter.
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
			// Skip the two mandatory OGG Opus header packets:
			// packet 0 = OpusHead (identification), packet 1 = OpusTags (comments).
			if headerCount < 2 {
				headerCount++
				continue
			}
			frameCh <- packet
		}
	}()

	p.vc.Speaking(true)
	ticker := time.NewTicker(20 * time.Millisecond)
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
				// Reader goroutine closed the channel — stream ended normally.
				break loop
			}
			// Pace delivery: wait for the next 20 ms tick before sending.
			select {
			case <-ticker.C:
				p.vc.OpusSend <- frame
				framesSent++
			case <-p.skip:
				fmt.Printf("%s Skipped after %d frames\n", label, framesSent)
				cmd.Process.Kill()
				skipped = true
				break loop
			}
		}
	}

	ticker.Stop()
	p.vc.Speaking(false)

	// Drain frameCh so the reader goroutine is never blocked trying to send.
	for range frameCh {
	}

	if err := cmd.Wait(); err != nil && !strings.Contains(err.Error(), "killed") {
		fmt.Printf("%s FFmpeg exited with error: %v\n", label, err)
	}

	// Print only the last few lines of FFmpeg stderr to avoid log flooding
	// (FFmpeg writes a progress line on every frame).
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

func queueSpotifySong(s1 *discordgo.Session, s2, s3 string) error {
	// Placeholder for future Spotify queue support.
	return nil
}

func playSpotifySong(s *discordgo.Session, guildID, channelID string) error {
	// Placeholder for future Spotify playback support.
	return nil
}

func stop(s *discordgo.Session, guildID string) error {
	// Placeholder for future stop logic.
	return nil
}
