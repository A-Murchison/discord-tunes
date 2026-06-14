# discord-tunes

A Discord music bot that streams YouTube audio directly into voice channels.

## Requirements

- [Go 1.21+](https://go.dev/dl/)
- [FFmpeg](https://www.gyan.dev/ffmpeg/builds/) (full build with `libopus`) in your PATH
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) in your PATH

## Setup

1. Copy `.env.example` to `.env.local` and fill in your bot token:

   ```
   DISCORD_TOKEN=your_bot_token_here
   ```

2. Build the bot:

   ```
   go build .
   ```

3. Set the environment variable and run:
   ```powershell
   # PowerShell — load .env.local and run
   Get-Content .env.local | ForEach-Object { $k,$v = $_ -split '=',2; [Environment]::SetEnvironmentVariable($k,$v) }
   .\discord-tunes.exe
   ```

## Commands

| Command                | Description                                                  |
| ---------------------- | ------------------------------------------------------------ |
| `!play <YouTube URL>`  | Add a track to the queue and start playing                   |
| `!queue <YouTube URL>` | Alias for `!play`                                            |
| `!skip`                | Skip the current track                                       |
| `!stop`                | Stop playback and clear the queue                            |
| `!clear`               | Clear the queue without stopping the current track           |
| `!test`                | Play a 5-second 440 Hz test tone (checks the voice pipeline) |
| `!diag`                | Report FFmpeg, libopus, and yt-dlp status                    |
| `!ping`                | Pong!                                                        |

## Dependencies

- [`github.com/bwmarrin/discordgo`](https://github.com/bwmarrin/discordgo) (replaced by [A-Murchison/discordgo-fork](https://github.com/A-Murchison/discordgo-fork))
- [`github.com/jonas747/ogg`](https://github.com/jonas747/ogg)
