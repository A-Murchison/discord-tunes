package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

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
	if _, err := os.Stat(".env.local"); err == nil {
		data, err := os.ReadFile(".env.local")
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading .env.local:", err)
			return
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])

			if key == "DISCORD_TOKEN" && os.Getenv("DISCORD_TOKEN") != "" {
				fmt.Fprintln(os.Stderr, "Warning: DISCORD_TOKEN is already set in the environment; ignoring value from .env.local")
				continue
			}
			fmt.Fprintf(os.Stderr, "Setting environment variable from .env.local: %s=***\n", key)
			value := strings.TrimSpace(parts[1])
			os.Setenv(key, value)
		}
	}
}

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "DISCORD_TOKEN environment variable is not set")
		os.Exit(1)
	}

	if err := initSpotify(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println(err)
	}

	discord.AddHandler(ready)
	discord.AddHandler(messageCreate)
	discord.AddHandler(guildCreate)

	discord.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentsMessageContent

	if err := initSpotify(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}

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
