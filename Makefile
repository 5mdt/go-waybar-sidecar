GO      := toolbox run go
BIN     := go-waybar-sidecar
BUILD   := build
PREFIX  := $(HOME)/.local/bin

.PHONY: all fmt test build install clean

all: fmt test build

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BUILD)/$(BIN) ./...

install: test build
	install -Dm755 $(BUILD)/$(BIN) $(PREFIX)/$(BIN)

clean:
	rm -rf $(BUILD)
