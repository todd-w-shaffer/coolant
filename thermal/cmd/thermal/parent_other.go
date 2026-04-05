//go:build !darwin

package main

// parentExitMsg is sent when the parent process dies.
// On non-darwin platforms, the watcher is a no-op.
type parentExitMsg struct{}

// watchParent is a no-op on non-darwin platforms.
// macOS uses kqueue EVFILT_PROC; Linux would need prctl(PR_SET_PDEATHSIG).
func watchParent(_ interface{ Send(msg any) }) {}
