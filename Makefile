.PHONY: build run setup check clean ensure-yt-dlp

BIN := discord-tunes
LOCAL_BIN := $(CURDIR)/.bin
YTDLP := $(LOCAL_BIN)/yt-dlp

build: ensure-yt-dlp
	go build -o $(BIN) .

run: ensure-yt-dlp
	PATH="$(LOCAL_BIN):$$PATH" go run .

setup: ensure-yt-dlp
	@command -v ffmpeg >/dev/null 2>&1 || { echo "ffmpeg is not installed or not in PATH"; exit 1; }
	@go version >/dev/null
	@echo "Setup OK"

check:
	@go version
	@ffmpeg -version | head -n 1
	@if command -v yt-dlp >/dev/null 2>&1; then yt-dlp --version; elif [ -x "$(YTDLP)" ]; then "$(YTDLP)" --version; else echo "yt-dlp not found"; exit 1; fi

ensure-yt-dlp:
	@if command -v yt-dlp >/dev/null 2>&1; then \
		echo "yt-dlp found: $$(command -v yt-dlp)"; \
	elif [ -x "$(YTDLP)" ]; then \
		echo "yt-dlp found: $(YTDLP)"; \
	else \
		echo "yt-dlp not found; downloading project-local copy to $(YTDLP)"; \
		mkdir -p "$(LOCAL_BIN)"; \
		curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o "$(YTDLP)"; \
		chmod +x "$(YTDLP)"; \
	fi

clean:
	rm -f $(BIN)
