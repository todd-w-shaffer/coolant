#!/usr/bin/env bats
#
# Integration tests for .githooks/pre-push, the belt layer that blocks
# pushes containing private content regardless of who (human or agent)
# initiated them.

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  setup_git_tmprepo
  make_bare_remote
  # Pre-populate origin/main so `origin/main..HEAD` resolves.
  git push -q origin main
}

# Helper: synthesize stdin for the pre-push hook and run it in the test
# repo. Positional args are pairs of (local_sha, remote_sha); local_ref
# defaults to refs/heads/main, remote_ref to refs/heads/main.
run_push_hook() {
  # Write directly into the pipe — `$(make_push_stdin ...)` strips the
  # trailing newline, which silently drops the last ref from the hook's
  # read loop under `set -e`.
  {
    while [ "$#" -gt 0 ]; do
      make_push_stdin refs/heads/main "$1" refs/heads/main "$2"
      shift 2
    done
  } | .githooks/pre-push
}

# Helper: commit a file and echo its new HEAD sha.
commit_file() {
  local path="$1" content="${2:-content}"
  mkdir -p "$(dirname "$path")"
  echo "$content" > "$path"
  git add "$path"
  git commit -q -m "add $path"
  git rev-parse HEAD
}

@test "pre-push exits 0 on empty stdin (no refs being pushed)" {
  run bash -c ': | .githooks/pre-push'
  [ "$status" -eq 0 ]
}

@test "pre-push allows a push of only allowlisted changes" {
  local remote_sha local_sha
  remote_sha=$(git rev-parse HEAD)
  local_sha=$(commit_file docs/go-design.md)
  run run_push_hook "$local_sha" "$remote_sha"
  [ "$status" -eq 0 ]
}

@test "pre-push blocks a push containing a blocklist-prefix path" {
  local remote_sha local_sha
  remote_sha=$(git rev-parse HEAD)
  local_sha=$(commit_file docs/backlog/foo.md)
  run run_push_hook "$local_sha" "$remote_sha"
  [ "$status" -eq 1 ]
  [[ "$output" == *"BLOCKED: docs/backlog/foo.md"* ]]
}

@test "pre-push handles branch deletion (local_sha all zeros)" {
  local zero=0000000000000000000000000000000000000000
  local remote_sha
  remote_sha=$(git rev-parse HEAD)
  run run_push_hook "$zero" "$remote_sha"
  [ "$status" -eq 0 ]
}

@test "pre-push handles new-branch case (remote_sha all zeros)" {
  local zero=0000000000000000000000000000000000000000
  local local_sha
  local_sha=$(commit_file docs/backlog/new.md)
  run run_push_hook "$local_sha" "$zero"
  [ "$status" -eq 1 ]
  [[ "$output" == *"BLOCKED: docs/backlog/new.md"* ]]
}

@test "pre-push handles merge commits" {
  local remote_sha base_sha feature_sha merge_sha
  remote_sha=$(git rev-parse HEAD)
  # Create a feature branch with an allowed file.
  git checkout -q -b feature
  feature_sha=$(commit_file docs/go-design.md "feature content")
  # Merge back into main.
  git checkout -q main
  git merge -q --no-ff --no-edit feature
  merge_sha=$(git rev-parse HEAD)
  run run_push_hook "$merge_sha" "$remote_sha"
  [ "$status" -eq 0 ]
}

@test "pre-push aggregates blocked paths across multiple refs in one push" {
  local remote_sha_main sha1 sha2
  remote_sha_main=$(git rev-parse HEAD)
  sha1=$(commit_file docs/backlog/a.md)
  # Create a second branch with another blocked path.
  git checkout -q -b feature "$remote_sha_main"
  sha2=$(commit_file docs/backlog/b.md)
  git checkout -q main

  run bash -c '
    {
      printf "refs/heads/main %s refs/heads/main %s\n" "$1" "$2"
      printf "refs/heads/feature %s refs/heads/feature %s\n" "$3" "0000000000000000000000000000000000000000"
    } | .githooks/pre-push
  ' _ "$sha1" "$remote_sha_main" "$sha2"
  [ "$status" -eq 1 ]
  [[ "$output" == *"2 path(s) blocked"* ]]
  [[ "$output" == *"docs/backlog/a.md"* ]]
  [[ "$output" == *"docs/backlog/b.md"* ]]
}

@test "pre-push --no-verify bypass works (integration)" {
  commit_file docs/backlog/leak.md >/dev/null
  # --no-verify bypasses the hook; bare-remote push succeeds.
  run git push --no-verify origin main
  [ "$status" -eq 0 ]
}

@test "pre-push survives force-push where remote_sha is unreachable" {
  local unreachable=0123456789abcdef0123456789abcdef01234567
  local local_sha
  local_sha=$(git rev-parse HEAD)
  run run_push_hook "$local_sha" "$unreachable"
  # Hook should not crash under set -e. Behavior: falls back to
  # origin/main..local_sha (no new content over origin/main), exits 0.
  [ "$status" -eq 0 ]
}

@test "pre-push handles paths with spaces in a pushed commit" {
  local remote_sha local_sha
  remote_sha=$(git rev-parse HEAD)
  local_sha=$(commit_file "docs/backlog/file with spaces.md")
  run run_push_hook "$local_sha" "$remote_sha"
  [ "$status" -eq 1 ]
  [[ "$output" == *"file with spaces.md"* ]]
}
