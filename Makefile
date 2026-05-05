BIN_DIR := $(HOME)/.local/bin

.PHONY: all build install clean

all: build

build:
	go build -o bin/git-sidebar ./cmd/git-sidebar

install: build
	mkdir -p $(BIN_DIR)
	cp bin/git-sidebar $(BIN_DIR)/git-sidebar

clean:
	rm -rf bin/
