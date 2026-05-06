# agen-tui

Tmux sidebar for working alongside Claude Code. Bubble Tea TUI showing git status, file tree, recent tool calls, and the active session's token + cost stats.

## Install

### Existing checkout

```
make install
```

Installs `agen-tui` to `~/.local/bin`.

### Fresh machine (one-shot)

Downloads the latest release binary, drops in the tmux sidebar script, and binds `prefix + g`. Idempotent. Works on Linux (apt/dnf/pacman) and macOS (brew). Falls back to a source build (Go required) when no release binary matches the platform.

```sh
curl -fsSL https://raw.githubusercontent.com/Booli/agen-tui/main/scripts/install.sh | bash
```

Add `--full` for sesh, fzf, zoxide, bat, micro:

```sh
curl -fsSL https://raw.githubusercontent.com/Booli/agen-tui/main/scripts/install.sh | bash -s -- --full
```

Force a source build:

```sh
curl -fsSL https://raw.githubusercontent.com/Booli/agen-tui/main/scripts/install.sh | bash -s -- --from-source
```

Make sure `$HOME/.local/bin` is on your `PATH` after.

### Cutting a release

Push a `v*` tag. The release workflow runs goreleaser and attaches `linux/darwin × amd64/arm64` tarballs to a GitHub Release; the install script picks them up automatically.

```sh
git tag v0.1.0
git push origin v0.1.0
```

## Use

Run from a git repo:

```
agen-tui
```

Or point it at a directory:

```
agen-tui /path/to/repo
```

### Remote (SSH)

Drive a repo on another machine while the TUI runs locally:

```
agen-tui user@host:/abs/path/to/repo
```

Git status, file tree, and Claude session JSONL are read over SSH on every refresh. Opening a file (`o`) splits a tmux pane on the local host that edits the remote file in place via vim's `scp://` protocol — no sync, no checkout. Best paired with an SSH `ControlMaster` so each call reuses the existing connection.

Cycle views with `g`, `t`, or `Tab`: `flat` -> `tree` -> `tools`.

## Tmux integration

Bind a toggle to prefix+g (or whatever):

```
bind g run-shell "~/path/to/sidebar.sh"
```

Sample `sidebar.sh`:

```bash
#!/usr/bin/env bash
PANE_TITLE="agen-tui"
PANE_PATH=$(tmux display-message -p "#{pane_current_path}")

existing=$(tmux list-panes -F "#{pane_id}:#{pane_title}" 2>/dev/null \
  | grep ":${PANE_TITLE}$" | cut -d: -f1)

if [[ -n "$existing" ]]; then
  tmux kill-pane -t "$existing"
  exit 0
fi

LOG="${TMPDIR:-/tmp}/agen-tui.log"
tmux split-window -h -l 70 -c "$PANE_PATH" \
  "tmux select-pane -T '${PANE_TITLE}'; exec '${HOME}/.local/bin/agen-tui' '$PANE_PATH' 2>>'${LOG}' || { echo \"agen-tui exited \$?, see ${LOG}\"; sleep 5; }"
```

## Keys

Global: `q` quit. `r` refresh. `g`/`t`/`Tab` cycle view.

Flat / Tree:

- `j`/`k` move
- `enter` open diff overlay
- `o` open file in main tmux pane via `$EDITOR`
- `esc` close overlay

Tools:

- `j`/`k` move
- `enter` open call detail
- `f` cycle filter (files / edits / all). Errors always show.
- `esc` close detail

## Layout

```
cmd/agen-tui/        bubbletea views (flat, tree, tools, file detail, tool detail)
internal/backend/    Backend interface; LocalBackend (filesystem) + SSHBackend (ssh)
internal/git/        porcelain status + diff helpers
internal/filetree/   file tree builder
internal/session/    JSONL parser, ToolCall, FilterMode
internal/ui/         text wrap, line diff, unified diff render
internal/theme/      lipgloss styles
```

## Test

```
go test ./...
```

## Dependencies

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles` (key, viewport)
- `github.com/charmbracelet/lipgloss`

## License

See `LICENSE`.
