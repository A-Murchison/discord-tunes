# discord-tunes

Discord music bot that streams YouTube or Spotify tunes directly into voice channels with DAVE (E2EE) https://daveprotocol.com/.

Built with Go. Made for fun.

## What it does

- Joins the voice channel you are currently in
- Plays YouTube URLs
- Resolves Spotify track/playlist/album URLs to playable tracks through YouTube search
- Queues tracks per Discord server
- Includes diagnostics for FFmpeg, `libopus`, and `yt-dlp`

## Requirements

- [Go 1.21+](https://go.dev/dl/)
- [FFmpeg](https://ffmpeg.org/download.html) with `libopus` support in your `PATH`
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) in your `PATH`
- A Discord bot token
- Spotify API credentials if you want Spotify URL support

> Note: `make build` and `make run` can download a project-local copy of `yt-dlp` into `.bin/` if `yt-dlp` is missing. 

## Commands

| Command                | Description                                                  |
| ---------------------- | ------------------------------------------------------------ |
| `!play <URL>`          | Add a YouTube or Spotify track to the queue and start playing |
| `!queue <URL>`         | Alias for `!play`                                            |
| `!playlist`            | Show the current queue                                       |
| `!skip`                | Skip the current track                                       |
| `!stop`                | Stop playback and clear the queue                            |
| `!clear`               | Clear the queue without stopping the current track            |
| `!test`                | Play a 5-second 440 Hz test tone                             |
| `!diag`                | Report FFmpeg, libopus, and yt-dlp status                    |
| `!ping`                | Pong!                                                        |


## Create and invite the Discord bot

1. Go to the Discord Developer Portal: <https://discord.com/developers/applications>
2. Create an application, then open **Bot**.
3. Copy the bot token.
4. Enable **Message Content Intent** under the bot's privileged gateway intents.
   - This is required because commands use text prefixes like `!play`.
5. Open **OAuth2 -> URL Generator**.
6. Select the `bot` scope.
7. Select these bot permissions:
   - View Channels
   - Send Messages
   - Read Message History
   - Connect
   - Speak
   - Use Voice Activity
8. Open the generated URL and invite the bot to your server.

The bot does not join a voice channel immediately. It joins the voice channel of the user who runs `!play` or `!test`.

## Configure environment variables

Copy the example environment file:

```bash
cp .env.example .env.local
```

Fill in your credentials:

```env
DISCORD_TOKEN=your_discord_bot_token_here
SPOTIFY_CLIENT_ID=your_spotify_client_id_here
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret_here
```

`DISCORD_TOKEN` is required. Spotify credentials are only required for Spotify links.

The app automatically loads `.env.local` on startup. Values already set in your shell take priority over `.env.local`.

## Using the bot

1. Join a Discord voice channel.
2. In a text channel the bot can read, test basic connectivity:

   ```text
   !ping
   ```

3. Test voice playback:

   ```text
   !test
   ```

4. Play a YouTube track:

   ```text
   !play https://www.youtube.com/watch?v=btPJPFnesV4
   ```

5. Play a Spotify link, if Spotify credentials are configured:

   ```text
   !play https://open.spotify.com/track/...
   ```

## Build and run

### Easiest path

Use the included `Makefile`:

```bash
make setup
make run
```

`make setup` checks Go and FFmpeg, and downloads `yt-dlp` to `.bin/yt-dlp` if no system `yt-dlp` is found.

To build a binary:

```bash
make build
```

Then run it:

```bash
PATH="$(pwd)/.bin:$PATH" ./discord-tunes
```

The `PATH` prefix is only needed if `yt-dlp` was installed into the project-local `.bin/` directory instead of system-wide.

### Manual Go commands

If all dependencies are already installed system-wide:

```bash
go build .
./discord-tunes
```

Or:

```bash
go run .
```

A successful startup looks like:

```text
Logged in as: yourbot#0000
Bot is now running. Press CTRL-C to exit.
Joined guild: Your Server
```

## Install dependencies

### FFmpeg

Linux DNF example:

```bash
sudo dnf update
sudo dnf install ffmpeg
```

Linux apt example:

```bash
sudo apt update
sudo apt install ffmpeg
```

macOS with Homebrew:

```bash
brew install ffmpeg
```

Windows with winget:

```powershell
winget install Gyan.FFmpeg
```

Windows with Chocolatey:

```powershell
choco install ffmpeg
```

After installing on Windows, open a new terminal so your updated `PATH` is loaded.

Verify FFmpeg and `libopus` support:

```bash
ffmpeg -version
ffmpeg -encoders | grep libopus
```

On Windows PowerShell, use:

```powershell
ffmpeg -encoders | Select-String libopus
```

### yt-dlp

Linux DNF example:

```bash
sudo dnf install yt-dlp
```

Linux apt example:

```bash
sudo apt update
sudo apt install yt-dlp
```

macOS with Homebrew:

```bash
brew install yt-dlp
```

Windows with winget:

```powershell
winget install yt-dlp.yt-dlp
```

Windows with Chocolatey:

```powershell
choco install yt-dlp
```

Standalone install, useful if your package manager has an old version:

```bash
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod +x /usr/local/bin/yt-dlp
```

Verify:

```bash
yt-dlp --version
```


## Troubleshooting

### `DISCORD_TOKEN environment variable is not set`

Create `.env.local` and set `DISCORD_TOKEN`, or export it in your shell:

```bash
export DISCORD_TOKEN=your_discord_bot_token_here
```

### Bot is online but does not respond to commands

Check that:

- The bot was invited to the server
- The bot can view/read/send messages in that channel
- **Message Content Intent** is enabled in the Discord Developer Portal
- You restarted the bot after changing portal settings

### `Error getting stream URL: yt-dlp --get-url: exec: "yt-dlp": executable file not found in $PATH`

Install `yt-dlp` system-wide, or run through `make run` so the project-local `.bin/yt-dlp` is added to `PATH`:

```bash
make run
```

If you already ran `make build` and it downloaded `.bin/yt-dlp`, run the binary like this:

```bash
PATH="$(pwd)/.bin:$PATH" ./discord-tunes
```

### Bot joins but no audio plays

Run:

```text
!diag
```

Make sure:

- FFmpeg is installed
- FFmpeg includes the `libopus` encoder
- `yt-dlp` is installed and visible in `PATH`
- The bot has **Connect** and **Speak** permissions in the voice channel

## Notes on auto-installing yt-dlp

Go itself does not have a safe build hook that runs arbitrary installers during `go build .`. This is intentional: building code should not unexpectedly modify the user's system.

This repo supports a safer compromise:

- `make build` checks for `yt-dlp`
- If missing, it downloads a local copy into `.bin/yt-dlp`
- `make run` automatically adds `.bin/` to `PATH`

If you deploy the compiled binary somewhere else, install `yt-dlp` on that machine or include `.bin/yt-dlp` and add it to `PATH` before starting the bot.

## Dependencies

- [`github.com/bwmarrin/discordgo`](https://github.com/bwmarrin/discordgo) replaced by [A-Murchison/discordgo-fork](https://github.com/A-Murchison/discordgo-fork)
- [`github.com/jonas747/ogg`](https://github.com/jonas747/ogg)

## Why not `dca`?

I tried to get this to work, I couldn't get it to work. Just a note for future contributors.

The common approach for Discord audio bots in Go is to use the [`dca`](https://github.com/bwmarrin/dca) library, which wraps FFmpeg and produces DCA-framed Opus audio. I evaluated it and chose not to use it for two reasons:

1. **Broken on FFmpeg 7+.** `dca` hardcodes `-vbr on` in its FFmpeg arguments. FFmpeg 7 changed VBR flag handling for `libopus`, causing `dca` to produce malformed or silent output on modern FFmpeg builds.
2. **Unnecessary abstraction.** `dca` exists to strip raw Opus packets out of FFmpeg output and frame them for Discord. The same result is achieved more simply by having FFmpeg encode directly to OGG, which is a standard Opus container, and decoding the OGG packets with [`github.com/jonas747/ogg`](https://github.com/jonas747/ogg). This gives us full control over FFmpeg arguments and removes a dependency with an active upstream bug.
