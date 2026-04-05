package main

import (
	"os"
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// parentExitMsg is sent when the parent process dies.
type parentExitMsg struct{}

// watchParent uses kqueue EVFILT_PROC+NOTE_EXIT to detect parent death
// without polling. When the parent exits, it sends parentExitMsg to the
// bubbletea program so the viz can shut down cleanly instead of becoming
// an orphan pinned to PID 1.
func watchParent(p *tea.Program) {
	ppid := os.Getppid()
	if ppid <= 1 {
		// Already orphaned or running under init — nothing to watch.
		return
	}

	kq, err := syscall.Kqueue()
	if err != nil {
		return
	}

	event := syscall.Kevent_t{
		Ident:  uint64(ppid),
		Filter: syscall.EVFILT_PROC,
		Flags:  syscall.EV_ADD | syscall.EV_ONESHOT,
		Fflags: syscall.NOTE_EXIT,
	}

	events := make([]syscall.Kevent_t, 1)
	// Blocks until the parent exits.
	n, err := syscall.Kevent(kq, []syscall.Kevent_t{event}, events, nil)
	syscall.Close(kq)
	if err != nil || n < 1 {
		return
	}

	p.Send(parentExitMsg{})
}
