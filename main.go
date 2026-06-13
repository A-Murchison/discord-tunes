// Init main package
package main

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var token string

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
	discord.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Open the websocket
	err = discord.Open()

	if err != nil {
		fmt.Println("Error opening Discord session:", err)
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")

	discord.Close()
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
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			fmt.Println("Error fetching channel:", err)
			return
		}

		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			// Could not find guild.
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				err = playSpotifySong(s, g.ID, vs.ChannelID)
				if err != nil {
					fmt.Println("Error playing sound:", err)
				}

				return
			}
		}
	}

	if strings.HasPrefix(m.Content, "!queueSong") {
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			fmt.Println("Error fetching channel:", err)
			return
		}

		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			// Could not find guild.
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				err = playSpotifySong(s, g.ID, vs.ChannelID)
				if err != nil {
					fmt.Println("Error playing sound:", err)
				}

				return
			}
		}
	}
}

func playSpotifySong(s *discordgo.Session, guildID, channelID string) error {
	// This is where you would implement the logic to play a Spotify song in the voice channel.
	// You would need to use a library that can handle audio streaming to Discord, such as "github.com/jonas747/dca".
	// You would also need to handle authentication with Spotify's API to fetch the song data.
	return nil
}
