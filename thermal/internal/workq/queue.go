package workq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultPoll is how often a waiter re-reads the queue. It is a sample
// rate, not a limit: no wait ends on time.
const DefaultPoll = 200 * time.Millisecond

// Queue is a handle on the queue directory. Each handle holds its own
// open file description of queue.lock, so two handles in one process
// exclude each other exactly as two processes would.
type Queue struct {
	Dir  string
	Poll time.Duration
	lock *os.File
}

// Open makes (if needed) and opens the queue directory. An error means
// the queue cannot be used; the caller degrades to running unqueued.
func Open(dir string) (*Queue, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "queue.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	return &Queue{Dir: dir, Poll: DefaultPoll, lock: f}, nil
}

// Close releases the handle.
func (q *Queue) Close() error { return q.lock.Close() }

func (q *Queue) withLock(fn func() error) error {
	if err := syscall.Flock(int(q.lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock queue: %w", err)
	}
	defer func() { _ = syscall.Flock(int(q.lock.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func (q *Queue) ticketPath(seq int) string {
	return filepath.Join(q.Dir, strconv.Itoa(seq)+".json")
}

// Hold is a taken ticket. The holder keeps flock on the ticket file
// until Release (or until the process dies, when the kernel drops it).
type Hold struct {
	Ticket Ticket
	// Waited is how long the ticket waited for admission (0 in observe).
	Waited time.Duration
	// WouldWait reports whether enforce mode would have held this job at
	// arrival, given what was running. Meaningful in observe mode.
	WouldWait bool
	// What else was running when this job started.
	OthersRunning int
	OthersCores   int
	OthersMemMB   int

	q *Queue
	f *os.File
}

// Release removes the ticket and drops its lock.
func (h *Hold) Release() {
	if h.f == nil {
		return
	}
	_ = h.q.withLock(func() error {
		_ = os.Remove(h.q.ticketPath(h.Ticket.Seq))
		return nil
	})
	_ = h.f.Close()
	h.f = nil
}

// Take puts t in the queue and answers once it may run. In observe mode
// (enforce false) it is admitted at once and only WouldWait is computed.
// In enforce mode it waits, with no time limit, until an admission pass
// admits it; onWait (may be nil) is called with the jobs ahead each time
// that set changes. Canceling ctx abandons the wait and removes the
// ticket.
func (q *Queue) Take(ctx context.Context, t Ticket, b Budget, enforce bool, onWait func(ahead []Ticket)) (*Hold, error) {
	h := &Hold{q: q}
	now := time.Now()
	t.QueuedAt = now.UTC().Format(time.RFC3339)
	t.State = StateWaiting
	err := q.withLock(func() error {
		tickets, err := q.scan()
		if err != nil {
			return err
		}
		seq, err := q.nextSeq(tickets)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(q.ticketPath(seq), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = f.Close()
			_ = os.Remove(q.ticketPath(seq))
			return err
		}
		t.Seq = seq
		h.f = f
		running, waiting := split(append(tickets, t))
		admitted := Admit(running, waiting, b)
		h.WouldWait = !containsSeq(admitted, seq)
		if !enforce {
			h.noteOthers(running, b)
			t.State = StateRunning
			t.StartedAt = t.QueuedAt
		}
		h.Ticket = t
		return writeTicket(f, t)
	})
	if err != nil {
		if h.f != nil {
			h.Release()
		}
		return nil, err
	}
	if !enforce {
		return h, nil
	}

	var lastAhead string
	for {
		admitted := false
		var ahead []Ticket
		err := q.withLock(func() error {
			tickets, err := q.scan()
			if err != nil {
				return err
			}
			if !containsSeq(tickets, h.Ticket.Seq) {
				return fmt.Errorf("ticket %d vanished from %s", h.Ticket.Seq, q.Dir)
			}
			tickets = q.admissionPass(tickets, b)
			running, waiting := split(tickets)
			for _, tk := range tickets {
				if tk.Seq == h.Ticket.Seq && tk.State == StateRunning {
					admitted = true
					h.Ticket = tk
					h.noteOthers(without(running, tk.Seq), b)
					return nil
				}
			}
			ahead = append(ahead, running...)
			for _, w := range waiting {
				if w.Seq == h.Ticket.Seq {
					break
				}
				ahead = append(ahead, w)
			}
			return nil
		})
		if err != nil {
			h.Release()
			return nil, err
		}
		if admitted {
			h.Waited = time.Since(now)
			return h, nil
		}
		if sig := signature(ahead); sig != lastAhead {
			lastAhead = sig
			if onWait != nil {
				onWait(ahead)
			}
		}
		select {
		case <-ctx.Done():
			h.Release()
			return nil, ctx.Err()
		case <-time.After(q.Poll):
		}
	}
}

func (h *Hold) noteOthers(running []Ticket, b Budget) {
	h.OthersRunning = len(running)
	h.OthersCores, h.OthersMemMB = 0, 0
	for _, r := range running {
		n := r.Need(b)
		h.OthersCores += n.Cores
		h.OthersMemMB += n.MemMB
	}
}

// admissionPass marks every admissible waiting ticket running, writing
// each on its owner's behalf. Admitting all of them in one pass (not
// only the caller) keeps a second waiter from spending the head's share
// before the head next polls. Caller holds the queue lock.
func (q *Queue) admissionPass(tickets []Ticket, b Budget) []Ticket {
	running, waiting := split(tickets)
	admitted := Admit(running, waiting, b)
	if len(admitted) == 0 {
		return tickets
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range tickets {
		if !containsSeq(admitted, tickets[i].Seq) {
			continue
		}
		tickets[i].State = StateRunning
		tickets[i].StartedAt = now
		f, err := os.OpenFile(q.ticketPath(tickets[i].Seq), os.O_RDWR, 0)
		if err != nil {
			continue
		}
		_ = writeTicket(f, tickets[i])
		_ = f.Close()
	}
	return tickets
}

// Snapshot answers the running tickets (by seq) and the waiting tickets
// in service order, removing any a dead holder left behind.
func (q *Queue) Snapshot() (running, waiting []Ticket, err error) {
	err = q.withLock(func() error {
		tickets, err := q.scan()
		if err != nil {
			return err
		}
		running, waiting = split(tickets)
		return nil
	})
	return running, waiting, err
}

// Live reports whether ticket seq exists, names pid, and is held by a
// live process (its lock is taken).
func (q *Queue) Live(seq, pid int) bool {
	live := false
	_ = q.withLock(func() error {
		f, err := os.Open(q.ticketPath(seq))
		if err != nil {
			return nil
		}
		defer f.Close()
		if syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) == nil {
			return nil // nobody holds it: the holder is dead
		}
		var t Ticket
		if json.NewDecoder(f).Decode(&t) == nil && t.PID == pid {
			live = true
		}
		return nil
	})
	return live
}

// scan reads every ticket, removing those whose holder is dead (lock
// free). Caller holds the queue lock. Tickets are created and locked
// under the queue lock, so an unlocked ticket is never a live one.
func (q *Queue) scan() ([]Ticket, error) {
	entries, err := os.ReadDir(q.Dir)
	if err != nil {
		return nil, err
	}
	var out []Ticket
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimSuffix(name, ".json")); err != nil {
			continue
		}
		path := filepath.Join(q.Dir, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			_ = os.Remove(path)
			_ = f.Close()
			continue
		}
		var t Ticket
		derr := json.NewDecoder(f).Decode(&t)
		_ = f.Close()
		if derr == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// nextSeq draws a ticket number that has never been used: the larger of
// the counter in `next` and one past the highest live ticket. Caller
// holds the queue lock.
func (q *Queue) nextSeq(tickets []Ticket) (int, error) {
	path := filepath.Join(q.Dir, "next")
	n := 1
	if b, err := os.ReadFile(path); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && v > n {
			n = v
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if len(tickets) > 0 && tickets[len(tickets)-1].Seq+1 > n {
		n = tickets[len(tickets)-1].Seq + 1
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(n+1)+"\n"), 0o600); err != nil {
		return 0, err
	}
	return n, nil
}

func writeTicket(f *os.File, t Ticket) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.WriteAt(append(b, '\n'), 0)
	return err
}

// split answers the running tickets (in seq order, as scanned) and the
// waiting tickets in service order.
func split(ts []Ticket) (running, waiting []Ticket) {
	for _, t := range ts {
		if t.State == StateRunning {
			running = append(running, t)
		}
	}
	return running, OrderWaiting(ts)
}

func without(ts []Ticket, seq int) []Ticket {
	var out []Ticket
	for _, t := range ts {
		if t.Seq != seq {
			out = append(out, t)
		}
	}
	return out
}

func containsSeq(ts []Ticket, seq int) bool {
	for _, t := range ts {
		if t.Seq == seq {
			return true
		}
	}
	return false
}

func signature(ts []Ticket) string {
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "%d:%s,", t.Seq, t.State)
	}
	return b.String()
}
