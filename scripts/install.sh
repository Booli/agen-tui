#!/usr/bin/env bash
# Installs agen-tui + the tmux sidebar binding on the current host.
# Idempotent: safe to re-run after upgrades. Defaults to a minimal
# footprint (just agen-tui, tmux, go); pass --full to also install
# sesh, fzf, zoxide, bat, micro.
#
# Usage:
#   bash scripts/install.sh             # minimal
#   bash scripts/install.sh --full      # everything
#   curl -fsSL https://raw.githubusercontent.com/Booli/agen-tui/main/scripts/install.sh | bash

set -euo pipefail

FULL=0
for arg in "$@"; do
    case "$arg" in
        --full) FULL=1 ;;
        --help|-h)
            sed -n '2,12p' "$0"
            exit 0
            ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

REPO_URL="https://github.com/Booli/agen-tui.git"
PREFIX="${HOME}/.local/bin"
SRC_DIR="${HOME}/.local/src/agen-tui"
TMUX_PLUGIN_DIR="${HOME}/.tmux/plugins/git-sidebar/scripts"
TMUX_CONF="${HOME}/.tmux.conf"

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m!!!\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

mkdir -p "$PREFIX" "$TMUX_PLUGIN_DIR" "$(dirname "$SRC_DIR")"

# ── Detect package manager ────────────────────────────────────────────────────
if have brew; then
    PM=brew
elif have apt-get; then
    PM=apt
elif have dnf; then
    PM=dnf
elif have pacman; then
    PM=pacman
else
    warn "no supported package manager found (brew/apt/dnf/pacman); install deps manually"
    PM=none
fi
log "package manager: $PM"

pkg_install() {
    case "$PM" in
        brew)   brew install "$@" ;;
        apt)    sudo apt-get install -y "$@" ;;
        dnf)    sudo dnf install -y "$@" ;;
        pacman) sudo pacman -S --needed --noconfirm "$@" ;;
        none)   warn "would install: $*" ;;
    esac
}

# ── Core deps ─────────────────────────────────────────────────────────────────
log "core deps (tmux, git, go)"
case "$PM" in
    apt) sudo apt-get update -y >/dev/null ;;
esac

have tmux || pkg_install tmux
have git  || pkg_install git
have go   || pkg_install golang-go || pkg_install go

# ── Clone or update repo ──────────────────────────────────────────────────────
if [ -d "$SRC_DIR/.git" ]; then
    log "updating $SRC_DIR"
    git -C "$SRC_DIR" pull --ff-only --quiet
else
    log "cloning $REPO_URL -> $SRC_DIR"
    git clone --quiet "$REPO_URL" "$SRC_DIR"
fi

# ── Build agen-tui ────────────────────────────────────────────────────────────
log "building agen-tui"
( cd "$SRC_DIR" && go build -o "$PREFIX/agen-tui" ./cmd/agen-tui )
log "installed: $PREFIX/agen-tui"

# ── Drop in sidebar.sh ────────────────────────────────────────────────────────
log "writing $TMUX_PLUGIN_DIR/sidebar.sh"
cat > "$TMUX_PLUGIN_DIR/sidebar.sh" <<'EOF'
#!/usr/bin/env bash
# Toggle the agen-tui sidebar pane (bound to prefix + g by default).

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
EOF
chmod +x "$TMUX_PLUGIN_DIR/sidebar.sh"

# ── Append tmux.conf binding (idempotent) ─────────────────────────────────────
MARKER="# >>> agen-tui (managed by install.sh) >>>"
END_MARKER="# <<< agen-tui <<<"
if [ -f "$TMUX_CONF" ] && grep -qF "$MARKER" "$TMUX_CONF"; then
    log "tmux.conf already has agen-tui block (skipping)"
else
    log "appending agen-tui block to $TMUX_CONF"
    {
        printf '\n%s\n' "$MARKER"
        printf 'bind g run-shell "%s/sidebar.sh"\n' "$TMUX_PLUGIN_DIR"
        printf '%s\n' "$END_MARKER"
    } >> "$TMUX_CONF"
fi

# ── Optional extras ───────────────────────────────────────────────────────────
if [ "$FULL" -eq 1 ]; then
    log "full mode: bat, micro, fzf, zoxide, sesh"
    have bat    || pkg_install bat || pkg_install batcat
    have micro  || pkg_install micro || warn "micro not available via $PM; download from https://micro-editor.github.io"
    have fzf    || pkg_install fzf
    have zoxide || pkg_install zoxide || warn "zoxide not available via $PM; see https://github.com/ajeetdsouza/zoxide"

    if ! have sesh; then
        case "$PM" in
            brew) brew install joshmedeski/sesh/sesh ;;
            *)
                log "downloading sesh release binary"
                ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
                OS=$(uname -s | tr '[:upper:]' '[:lower:]')
                LATEST=$(curl -fsSL https://api.github.com/repos/joshmedeski/sesh/releases/latest \
                          | grep '"tag_name"' | cut -d'"' -f4)
                BARE="${LATEST#v}"
                URL="https://github.com/joshmedeski/sesh/releases/download/${LATEST}/sesh_${BARE}_${OS}_${ARCH}.tar.gz"
                curl -fsSL "$URL" -o /tmp/sesh.tar.gz
                tar -xzf /tmp/sesh.tar.gz -C /tmp sesh
                install -m 755 /tmp/sesh "$PREFIX/sesh"
                rm -f /tmp/sesh.tar.gz /tmp/sesh
                ;;
        esac
    fi
fi

# ── Live reload if we're inside tmux ──────────────────────────────────────────
if [ -n "${TMUX:-}" ]; then
    tmux source-file "$TMUX_CONF" 2>/dev/null || true
    log "tmux config reloaded"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
cat <<EOF

agen-tui ready.

  binary       $PREFIX/agen-tui
  sidebar      $TMUX_PLUGIN_DIR/sidebar.sh
  toggle key   tmux prefix + g

If $PREFIX is not in your PATH, add this to your shell rc:

  export PATH="\$HOME/.local/bin:\$PATH"

EOF
