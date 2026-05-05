# agen-tui

Tmux sidebar for working alongside Claude Code. Bubble Tea TUI showing git status, file tree, recent tool calls, and the active session's token + cost stats.

## Install

```
make install
```

Installs `agen-tui` to `~/.local/bin`.

## Use

Run from a git repo:

```
agen-tui
```

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
cmd/agen-tui/    bubbletea views (flat, tree, tools, file detail, tool detail)
internal/git/    porcelain status + diff helpers
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
