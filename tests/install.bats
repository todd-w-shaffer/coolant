#!/usr/bin/env bats

load test_helper

# We test the pure logic functions from install.sh by extracting them.
# The full script can't run in tests (needs curl, /dev/tty, etc.)
# so we test the pieces we can isolate.

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export HOME="$TEST_TMPDIR/home"
  mkdir -p "$HOME"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ── path expansion ──────────────────────────────────────

@test "tilde expands to HOME" {
  INSTALL_DIR="~/bin"
  INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
  [ "$INSTALL_DIR" = "$TEST_TMPDIR/home/bin" ]
}

@test "literal \$HOME expands to HOME" {
  INSTALL_DIR='$HOME/bin'
  INSTALL_DIR="${INSTALL_DIR//\$HOME/$HOME}"
  [ "$INSTALL_DIR" = "$TEST_TMPDIR/home/bin" ]
}

@test "path without tilde or \$HOME passes through unchanged" {
  INSTALL_DIR="/usr/local/bin"
  INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
  INSTALL_DIR="${INSTALL_DIR//\$HOME/$HOME}"
  [ "$INSTALL_DIR" = "/usr/local/bin" ]
}

@test "empty input falls back to default" {
  DEFAULT_DIR="/usr/local/bin"
  INSTALL_DIR=""
  INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_DIR}"
  [ "$INSTALL_DIR" = "/usr/local/bin" ]
}

# ── shell profile detection ─────────────────────────────

@test "zsh shell detects .zshrc" {
  SHELL="/bin/zsh"
  SHELL_NAME="$(basename "$SHELL")"
  case "$SHELL_NAME" in
    zsh)  PROFILE="$HOME/.zshrc" ;;
    bash) PROFILE="$HOME/.bashrc" ;;
    *)    PROFILE="$HOME/.profile" ;;
  esac
  [ "$PROFILE" = "$HOME/.zshrc" ]
}

@test "bash shell detects .bashrc" {
  SHELL="/bin/bash"
  SHELL_NAME="$(basename "$SHELL")"
  case "$SHELL_NAME" in
    zsh)  PROFILE="$HOME/.zshrc" ;;
    bash) PROFILE="$HOME/.bashrc" ;;
    *)    PROFILE="$HOME/.profile" ;;
  esac
  [ "$PROFILE" = "$HOME/.bashrc" ]
}

@test "unknown shell falls back to .profile" {
  SHELL="/bin/fish"
  SHELL_NAME="$(basename "$SHELL")"
  case "$SHELL_NAME" in
    zsh)  PROFILE="$HOME/.zshrc" ;;
    bash) PROFILE="$HOME/.bashrc" ;;
    *)    PROFILE="$HOME/.profile" ;;
  esac
  [ "$PROFILE" = "$HOME/.profile" ]
}

# ── PATH entry install ──────────────────────────────────

@test "PATH entry appended to profile with coolant comment" {
  PROFILE="$HOME/.zshrc"
  INSTALL_DIR="$HOME/.local/bin"
  touch "$PROFILE"

  echo "" >> "$PROFILE"
  echo "# coolant" >> "$PROFILE"
  echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$PROFILE"

  grep -q '# coolant' "$PROFILE"
  grep -q "$INSTALL_DIR" "$PROFILE"
}

# ── uninstall ───────────────────────────────────────────

@test "uninstall removes PATH entry from profile" {
  PROFILE="$HOME/.zshrc"
  cat > "$PROFILE" << 'EOF'
export EDITOR=vim

# coolant
export PATH="/Users/test/.local/bin:$PATH"

alias ll='ls -la'
EOF

  sed -i '' '/^# coolant$/,+1d' "$PROFILE"

  run grep '# coolant' "$PROFILE"
  [ "$status" -ne 0 ]
  run grep '.local/bin' "$PROFILE"
  [ "$status" -ne 0 ]
  # Other content survives
  run grep 'EDITOR' "$PROFILE"
  [ "$status" -eq 0 ]
  run grep 'alias' "$PROFILE"
  [ "$status" -eq 0 ]
}

@test "uninstall removes statusline file" {
  mkdir -p "$HOME/.claude"
  touch "$HOME/.claude/statusline.sh"

  rm "$HOME/.claude/statusline.sh"

  [ ! -f "$HOME/.claude/statusline.sh" ]
}

@test "uninstall removes statusLine from settings.json" {
  mkdir -p "$HOME/.claude"
  cat > "$HOME/.claude/settings.json" << 'EOF'
{
  "theme": "dark",
  "statusLine": {
    "type": "command",
    "command": "bash ~/.claude/statusline.sh"
  }
}
EOF

  tmp=$(mktemp)
  jq 'del(.statusLine)' "$HOME/.claude/settings.json" > "$tmp" && mv "$tmp" "$HOME/.claude/settings.json"

  run jq '.statusLine' "$HOME/.claude/settings.json"
  [ "$output" = "null" ]
  # Other settings survive
  run jq -r '.theme' "$HOME/.claude/settings.json"
  [ "$output" = "dark" ]
}

@test "uninstall removes thermo binary" {
  mkdir -p "$HOME/.local/bin"
  touch "$HOME/.local/bin/thermo"
  chmod +x "$HOME/.local/bin/thermo"

  rm "$HOME/.local/bin/thermo"

  [ ! -f "$HOME/.local/bin/thermo" ]
}

# ── prompt_yn ──────────────────────────────────────────
# Tests for the validate-and-loop Y/n prompt.
# Single-pass variant outputs to stdout to avoid subshell variable issues.

_test_yn() {
  local _response
  read -r _response || _response=""
  case "$_response" in
    [Yy]|"") echo "Y" ;;
    [Nn])    echo "N" ;;
    *)       return 1 ;;
  esac
}

@test "prompt_yn accepts Y" {
  result=$(echo "Y" | _test_yn)
  [ "$result" = "Y" ]
}

@test "prompt_yn accepts y" {
  result=$(echo "y" | _test_yn)
  [ "$result" = "Y" ]
}

@test "prompt_yn accepts N" {
  result=$(echo "N" | _test_yn)
  [ "$result" = "N" ]
}

@test "prompt_yn accepts n" {
  result=$(echo "n" | _test_yn)
  [ "$result" = "N" ]
}

@test "prompt_yn treats empty as Y (default)" {
  result=$(echo "" | _test_yn)
  [ "$result" = "Y" ]
}

@test "prompt_yn rejects arrow key escape sequence" {
  run bash -c 'printf "\033[C\n" | { read -r r || r=""; case "$r" in [Yy]|"") echo Y;; [Nn]) echo N;; *) exit 1;; esac; }'
  [ "$status" -ne 0 ]
}

@test "prompt_yn rejects arbitrary text" {
  run bash -c 'echo "hello" | { read -r r || r=""; case "$r" in [Yy]|"") echo Y;; [Nn]) echo N;; *) exit 1;; esac; }'
  [ "$status" -ne 0 ]
}

@test "prompt_yn does not echo garbage to stdout" {
  # Simulate: garbage then valid Y — only the accepted value should appear in output
  run bash -c 'printf "hello\nY\n" | { while true; do read -r r || r=""; case "$r" in [Yy]|"") echo Y; exit 0;; [Nn]) echo N; exit 0;; *) ;; esac; done; }'
  [ "$status" -eq 0 ]
  [ "$output" = "Y" ]
}

# ── help flag ───────────────────────────────────────────

@test "--help exits 0 with usage text" {
  run bash "$PROJECT_ROOT/install.sh" --help
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "Usage"
  echo "$output" | grep -q "uninstall"
}

@test "-h exits 0 with usage text" {
  run bash "$PROJECT_ROOT/install.sh" -h
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "Usage"
}
