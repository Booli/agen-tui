# agen-tui

Tmux sidebar for working alongside Claude Code. Bubble Tea TUI showing git
status, a file tree, recent tool calls, the active session's token + cost
stats, and a manager for SSH local-forward tunnels — all in a single
narrow pane.

## Install

### Existing checkout

```
make install
```

Installs `agen-tui` to `~/.local/bin`.

### Fresh machine (one-shot)

Downloads the latest release binary, drops in the tmux sidebar script, and
binds `prefix + g`. Idempotent. Works on Linux (apt/dnf/pacman) and macOS
(brew). Falls back to a source build (Go required) when no release binary
matches the platform.

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

Push a `v*` tag. The release workflow runs goreleaser and attaches
`linux/darwin × amd64/arm64` tarballs to a GitHub Release; the install
script picks them up automatically.

```sh
git tag v0.2.0
git push origin v0.2.0
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

Git status, file tree, and Claude session JSONL are streamed over SSH.
Opening a file (`o`) splits a tmux pane that runs `ssh -t host vim '<remote
path>'` — no checkout, no sync. Best paired with an SSH `ControlMaster` so
each call reuses one connection.

### SSH tunnels

You can attach `ssh -L`-style local forwards on the command line:

```
agen-tui -L 5137 -L 8080:db:5432 user@host:/path
```

Or manage them interactively from the **tunnels view** (4th tab, always
reachable via `4`). Press `a` to add a forward, `d` to delete, `space` to
toggle.

Tunnels live in a per-user **registry** at `~/.config/agen-tui/tunnels.json`
(mode 0600), so they:

- **Persist** across agen-tui exits — closing the app doesn't kill the
  tunnel.
- **Are visible from any agen-tui instance** on the machine — adding `3000`
  in one terminal shows up in another within a couple seconds.
- **Are bound by host** — in remote mode the input takes just `PORT`, in
  local mode `HOST PORT` (e.g. `myserver 3000`).

The status of every tunnel is shown in a strip above the Claude stats:
`tunnels [host] 5137 ↑ 8080 …`. Stopped tunnels stay in the registry as
definitions — toggle with `space` to restart with the same spec.

### Fuzzy search

Press `/` in the flat, tree, or tools view to filter live with a fuzzy
subsequence match (fzf-style: bonuses for start of string, after word
boundaries, and consecutive matches). Diacritic-insensitive: typing `cafe`
matches `café`.

- Flat: filters the git-status list by file path.
- Tree: switches to a flat ranked list of matching files (preserves
  repo-wide search).
- Tools: filters tool calls by name + summary, with matched runes
  highlighted inline.

`enter` keeps the filter and resumes navigation; `esc` clears it.

## Configuration

Optional. Drop a JSON file at `~/.config/agen-tui/config.json` (or point
`$AGEN_TUI_CONFIG` at one; `$XDG_CONFIG_HOME` is honored). Any key you omit
keeps its default, so a partial file is fine.

```json
{
  "showIcons": true,
  "collapseFolders": false
}
```

- `showIcons` (default `false`) — turn on Nerd Font tree decorations:
  open/close chevrons, folder and file device-icons, and small git-status
  glyphs (`●` `○` `✕` `→` …). Requires a Nerd Font in your terminal. When
  off, the tree keeps its plain `▶`/`▼` arrows and letter status codes.
- `collapseFolders` (default `false`) — start the tree with every folder
  collapsed. When off (the default), top-level folders and the ancestors of
  any changed file are expanded on first load.

A missing file uses the defaults; a malformed file logs a warning to stderr
and falls back to the defaults.

## Tmux integration

Bind a toggle to `prefix + g`:

```tmux
bind g run-shell "~/.tmux/plugins/git-sidebar/scripts/sidebar.sh"
```

The sidebar script detects whether the current pane is `ssh`'d and either
launches agen-tui locally against the current pane's path, or against
`host:remote-path` when you're inside an SSH session whose remote shell is
also in tmux. See `scripts/sidebar.sh` in this repo for a reference
implementation.

## Keys

Global:

- `1` / `2` / `3` / `4` — jump directly to flat / tree / tools / tunnels.
- `t` or `Tab` / `Shift+Tab` — cycle views.
- `q` — quit.
- `r` — refresh git status.
- `T` — toggle all tunnels (start every stopped, or stop every up).

Flat / Tree:

- `j` / `k` move
- `enter` open diff (flat) / file view (tree)
- `o` open in `$EDITOR` via a split tmux pane
- `/` fuzzy search
- `esc` close overlay

Tools:

- `j` / `k` move
- `enter` open call detail
- `f` cycle filter (files / edits / all). Errors always show.
- `/` fuzzy search across name + summary
- `esc` close detail

Tunnels:

- `j` / `k` move
- `a` add (port / host port)
- `d` delete
- `space` / `enter` toggle the selected tunnel

## Layout

```
cmd/agen-tui/        bubbletea views (flat, tree, tools, tunnels, file/tool detail, chrome)
internal/backend/    Backend interface; LocalBackend (filesystem) + SSHBackend (ssh)
internal/git/        porcelain status + diff helpers
internal/filetree/   file tree builder
internal/session/    JSONL parser (incremental), ToolCall, FilterMode
internal/tunnel/     Spec + Registry (~/.config/agen-tui/tunnels.json) + detached spawn/kill
internal/fuzzy/      hand-rolled subsequence matcher with fzf-style scoring
internal/ui/         text wrap, line diff, unified diff render
internal/theme/      lipgloss styles
```

## Test

```
go test ./...
go test -race ./...
```

## Dependencies

Direct:

- `github.com/charmbracelet/bubbletea` — TUI event loop and alt-screen
- `github.com/charmbracelet/bubbles` — key bindings, viewport
- `github.com/charmbracelet/lipgloss` — styling
- `github.com/alecthomas/chroma/v2` — syntax highlighting in file view
- `golang.org/x/text` — Unicode normalization for the fuzzy matcher

The fuzzy matcher's diacritic-folding approach is borrowed from
[lithammer/fuzzysearch](https://github.com/lithammer/fuzzysearch) (MIT).

## License

See `LICENSE`.
