GO      := toolbox run go
BIN     := go-waybar-sidecar
BUILD   := build
VERSION := $(shell git describe --tags --always --dirty --match 'v*')
PREFIX  := $(HOME)/.local/bin

.PHONY: all fmt test build install clean

all: fmt test build

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BUILD)/$(BIN) ./...

install: test build
	install -Dm755 $(BUILD)/$(BIN) $(PREFIX)/$(BIN)

clean:
	rm -rf $(BUILD)
