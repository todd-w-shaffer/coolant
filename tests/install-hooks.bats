#!/usr/bin/env bats
#
# Tests for scripts/install-hooks.sh, the contributor bootstrap that
# sets core.hooksPath=.githooks and sanity-checks pre-existing state.

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export HOME="$TEST_TMPDIR/home"
  mkdir -p "$HOME"
  export GIT_CONFIG_GLOBAL=/dev/null
  export GIT_CONFIG_SYSTEM=/dev/null
  git init -q -b main "$TEST_TMPDIR/repo"
  cd "$TEST_TMPDIR/repo"
  mkdir -p .githooks .claude/hooks scripts
  # Copy install-hooks.sh in; symlink the rest so unrelated state isn't
  # left behind if a test touches files.
  cp "$PROJECT_ROOT/scripts/install-hooks.sh" scripts/install-hooks.sh
  chmod +x scripts/install-hooks.sh
  touch .githooks/pre-push
  touch .claude/hooks/classify-staged.sh
}

@test "install-hooks sets core.hooksPath" {
  run scripts/install-hooks.sh
  [ "$status" -eq 0 ]
  [ "$(git config --get core.hooksPath)" = ".githooks" ]
}

@test "install-hooks is idempotent" {
  run scripts/install-hooks.sh
  [ "$status" -eq 0 ]
  run scripts/install-hooks.sh
  [ "$status" -eq 0 ]
  [[ "$output" == *"already configured"* ]]
}

@test "install-hooks refuses to overwrite a non-.githooks value" {
  git config core.hooksPath custom/
  run scripts/install-hooks.sh
  [ "$status" -eq 1 ]
  [[ "$output" == *"custom/"* ]]
  [ "$(git config --get core.hooksPath)" = "custom/" ]
}

@test "install-hooks sets exec bit on hook scripts" {
  run scripts/install-hooks.sh
  [ "$status" -eq 0 ]
  [ -x .githooks/pre-push ]
  [ -x .claude/hooks/classify-staged.sh ]
}

@test "install-hooks fails cleanly outside a git repo" {
  cd "$TEST_TMPDIR"
  cp "$PROJECT_ROOT/scripts/install-hooks.sh" install-hooks.sh
  chmod +x install-hooks.sh
  run ./install-hooks.sh
  [ "$status" -ne 0 ]
}

@test "install-hooks warns about existing .git/hooks/ files" {
  echo "#!/bin/sh" > .git/hooks/post-commit
  chmod +x .git/hooks/post-commit
  run scripts/install-hooks.sh
  [ "$status" -eq 1 ]
  [[ "$output" == *"post-commit"* ]]
  [ -z "$(git config --get core.hooksPath || true)" ]
}

@test "install-hooks --force with typed MIGRATE proceeds past warning" {
  echo "#!/bin/sh" > .git/hooks/post-commit
  chmod +x .git/hooks/post-commit
  run bash -c 'echo MIGRATE | scripts/install-hooks.sh --force'
  [ "$status" -eq 0 ]
  [ "$(git config --get core.hooksPath)" = ".githooks" ]
}

@test "install-hooks --force rejects wrong typed confirmation" {
  echo "#!/bin/sh" > .git/hooks/post-commit
  chmod +x .git/hooks/post-commit
  run bash -c 'echo yes | scripts/install-hooks.sh --force'
  [ "$status" -ne 0 ]
  [ -z "$(git config --get core.hooksPath || true)" ]
}
