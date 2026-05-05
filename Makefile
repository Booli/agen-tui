BIN_DIR := $(HOME)/.local/bin

.PHONY: all build install clean

all: build

build:
	go build -o bin/agen-tui ./cmd/agen-tui

install: build
	mkdir -p $(BIN_DIR)
	cp bin/agen-tui $(BIN_DIR)/agen-tui

clean:
	rm -rf bin/
