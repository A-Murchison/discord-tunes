# discord-tunes

Discord music bot that streams YouTube or Spotify tunes directly into voice channels.

Built with go.

## Steps to play some tunes

Join a voice channel

1. !play https://www.youtube.com/watch?v=btPJPFnesV4
2. jam out

## Requirements

- [Go 1.21+](https://go.dev/dl/)
- [FFmpeg](https://www.gyan.dev/ffmpeg/builds/) (full build with `libopus`) in your PATH
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) in your PATH

## Setup

1. Copy `.env.example` to `.env.local` and fill in your bot token:

   ```
    DISCORD_TOKEN=your_bot_token_here
    SPOTIFY_CLIENT_ID=your_spotify_client_id_here
    SPOTIFY_CLIENT_SECRET=your_spotify_client_secret_here
   ```

2. Build the bot:

   ```
   go build .
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

## Why not `dca`?

I tried to get this to work, I couldn't get it to work. Just a note for future contributors.

The common approach for Discord audio bots in Go is to use the
[`dca`](https://github.com/bwmarrin/dca) library, which wraps FFmpeg and produces
DCA-framed Opus audio. I evaluated it and chose not to use it for two reasons:

1. **Broken on FFmpeg 7+.** `dca` hardcodes `-vbr on` in its FFmpeg arguments. FFmpeg 7
   changed VBR flag handling for `libopus`, causing `dca` to produce malformed or silent
   output on modern FFmpeg builds.

2. **Unnecessary abstraction.** `dca` exists to strip raw Opus packets out of FFmpeg
   output and frame them for Discord. The same result is achieved more simply by having
   FFmpeg encode directly to OGG (which is a standard Opus container) and decoding the
   OGG packets with [`github.com/jonas747/ogg`](https://github.com/jonas747/ogg). This
   gives us full control over FFmpeg arguments and removes a dependency with an active
   upstream bug.
