package workq

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain doubles as a helper process: WORKQ_HELPER_DIR set means
// "take a running ticket in that queue, say so, and hold it until killed".
func TestMain(m *testing.M) {
	// WORKQ_DOOR_HELPER set means "be a nested door": run the arguments
	// after `--` through Run, as `thermo run` would.
	if os.Getenv("WORKQ_DOOR_HELPER") != "" {
		args := os.Args
		for i, a := range args {
			if a == "--" {
				args = args[i+1:]
				break
			}
		}
		os.Exit(Run(context.Background(), Settings{
			QueueDir:    os.Getenv("WORKQ_HELPER_QUEUE"),
			RecordsPath: os.Getenv("WORKQ_HELPER_RECORDS"),
			Mode:        ModeEnforce,
			Budget:      budget8,
			Classes:     DefaultClassesForTest(),
			Env:         os.Environ(),
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}, args))
	}
	if dir := os.Getenv("WORKQ_HELPER_DIR"); dir != "" {
		q, err := Open(dir)
		if err != nil {
			fmt.Println("error", err)
			os.Exit(1)
		}
		h, err := q.Take(context.Background(), Ticket{PID: os.Getpid(), Class: "suite", Cores: 8, MemMB: 1000, Whole: true}, budget8, true, nil)
		if err != nil {
			fmt.Println("error", err)
			os.Exit(1)
		}
		fmt.Println("held", h.Ticket.Seq)
		select {}
	}
	os.Exit(m.Run())
}

func openQ(t *testing.T, dir string) *Queue {
	t.Helper()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q.Poll = 10 * time.Millisecond
	t.Cleanup(func() { q.Close() })
	return q
}

func job(class string, prio, cores, mem int, whole bool) Ticket {
	return Ticket{PID: os.Getpid(), Class: class, Priority: prio, Cores: cores, MemMB: mem, Whole: whole, Cwd: "/w/" + class}
}

func suiteJob(prio int) Ticket { return job("suite", prio, 8, 3000, true) }

type taken struct {
	h   *Hold
	err error
}

// takeAsync takes a ticket in enforce mode on a goroutine. At test end
// it cancels a wait still in progress and drains the goroutine, so no
// waiter outlives the queue handle it uses.
func takeAsync(tb testing.TB, q *Queue, t Ticket, onWait func([]Ticket)) chan taken {
	ch := make(chan taken, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h, err := q.Take(ctx, t, budget8, true, onWait)
		ch <- taken{h, err}
	}()
	tb.Cleanup(func() { cancel(); <-done })
	return ch
}

func mustHold(t *testing.T, ch chan taken) *Hold {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("take: %v", r.err)
		}
		return r.h
	case <-time.After(3 * time.Second):
		t.Fatal("take did not return")
	}
	return nil
}

func mustWait(t *testing.T, ch chan taken) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("take returned (seq %v, err %v), want it still waiting", r.h, r.err)
	case <-time.After(150 * time.Millisecond):
	}
}

func waitForWaiters(t *testing.T, q *Queue, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, w, err := q.Snapshot()
		if err == nil && len(w) == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("never saw %d waiters", n)
}

func TestEnforceAdmitsWhenBudgetFits(t *testing.T) {
	q := openQ(t, t.TempDir())
	h := mustHold(t, takeAsync(t, q, job("files", PriorityLane, 2, 1000, false), nil))
	if h.Ticket.State != StateRunning || h.Ticket.Seq == 0 {
		t.Fatalf("ticket = %+v", h.Ticket)
	}
	defer h.Release()
	h2 := mustHold(t, takeAsync(t, q, job("files", PriorityLane, 2, 1000, false), nil))
	defer h2.Release()
}

func TestEnforceHoldsWhileBudgetFullThenAdmitsWhenHolderEnds(t *testing.T) {
	dir := t.TempDir()
	a, b := openQ(t, dir), openQ(t, dir)
	first := mustHold(t, takeAsync(t, a, suiteJob(PriorityLane), nil))
	ch := takeAsync(t, b, job("typecheck", PriorityLane, 1, 1000, false), nil)
	mustWait(t, ch)
	first.Release()
	second := mustHold(t, ch)
	defer second.Release()
	if second.Waited <= 0 {
		t.Errorf("Waited = %v, want > 0", second.Waited)
	}
}

func TestWaitersAreServedFirstComeFirstServed(t *testing.T) {
	dir := t.TempDir()
	head := mustHold(t, takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil))
	var order []string
	chans := []chan taken{}
	for i := range 3 {
		j := suiteJob(PriorityLane)
		j.Cwd = fmt.Sprintf("/w/%d", i)
		chans = append(chans, takeAsync(t, openQ(t, dir), j, nil))
		waitForWaiters(t, openQ(t, dir), i+1)
	}
	head.Release()
	for i := range chans {
		h := mustHold(t, chans[i])
		if i+1 < len(chans) {
			mustWait(t, chans[i+1])
		}
		order = append(order, h.Ticket.Cwd)
		h.Release()
	}
	if !reflect.DeepEqual(order, []string{"/w/0", "/w/1", "/w/2"}) {
		t.Fatalf("order = %v", order)
	}
}

func TestInteractiveJumpsAheadOfEarlierLaneWaiter(t *testing.T) {
	dir := t.TempDir()
	head := mustHold(t, takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil))
	lane := takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil)
	waitForWaiters(t, openQ(t, dir), 1)
	inter := takeAsync(t, openQ(t, dir), suiteJob(PriorityInteractive), nil)
	waitForWaiters(t, openQ(t, dir), 2)
	// The running job is never stopped: both still wait.
	mustWait(t, inter)
	head.Release()
	h := mustHold(t, inter)
	mustWait(t, lane)
	h.Release()
	mustHold(t, lane).Release()
}

func TestDeadHolderLeavesTheQueue(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "WORKQ_HELPER_DIR="+dir)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	if !strings.HasPrefix(line, "held") {
		_ = cmd.Process.Kill()
		t.Fatalf("helper said %q", line)
	}
	ch := takeAsync(t, openQ(t, dir), job("typecheck", PriorityInteractive, 1, 500, false), nil)
	mustWait(t, ch)
	_ = cmd.Process.Signal(syscall.SIGKILL) // no cleanup runs: the kernel drops the flock
	_ = cmd.Wait()
	mustHold(t, ch).Release()
	entries, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(entries) != 0 {
		t.Errorf("tickets left behind: %v", entries)
	}
}

func TestWaitLinePrintsAgainEachTimeTheQueueMoves(t *testing.T) {
	dir := t.TempDir()
	a := mustHold(t, takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil))
	b := takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil)
	waitForWaiters(t, openQ(t, dir), 1)
	var mu sync.Mutex
	var lines [][]Ticket
	c := takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), func(ahead []Ticket) {
		mu.Lock()
		lines = append(lines, ahead)
		mu.Unlock()
	})
	mustWait(t, c)
	a.Release()
	hb := mustHold(t, b)
	mustWait(t, c)
	hb.Release()
	mustHold(t, c).Release()
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 2 {
		t.Fatalf("wait lines = %d, want 2 (initial + one move)", len(lines))
	}
	if len(lines[0]) != 2 || len(lines[1]) != 1 {
		t.Errorf("ahead sizes = %d then %d, want 2 then 1", len(lines[0]), len(lines[1]))
	}
}

func TestObserveModeNeverWaitsButRecordsWouldWait(t *testing.T) {
	dir := t.TempDir()
	q := openQ(t, dir)
	first, err := q.Take(context.Background(), suiteJob(PriorityLane), budget8, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if first.WouldWait {
		t.Error("first job on an empty machine would not wait")
	}
	second, err := openQ(t, dir).Take(context.Background(), suiteJob(PriorityLane), budget8, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	if second.Ticket.State != StateRunning {
		t.Errorf("observe ticket state = %s, want running", second.Ticket.State)
	}
	if !second.WouldWait {
		t.Error("second suite beside a running suite would have waited under enforce")
	}
	if second.OthersRunning != 1 || second.OthersCores != 8 {
		t.Errorf("others = %d jobs / %d cores, want 1 / 8", second.OthersRunning, second.OthersCores)
	}
}

func TestCanceledWaitRemovesItsTicket(t *testing.T) {
	dir := t.TempDir()
	head := mustHold(t, takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil))
	defer head.Release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := openQ(t, dir).Take(ctx, suiteJob(PriorityLane), budget8, true, nil)
		done <- err
	}()
	waitForWaiters(t, openQ(t, dir), 1)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled take returned no error")
	}
	waitForWaiters(t, openQ(t, dir), 0)
}

func TestSnapshotListsRunningAndWaitingInOrder(t *testing.T) {
	dir := t.TempDir()
	head := mustHold(t, takeAsync(t, openQ(t, dir), suiteJob(PriorityLane), nil))
	defer head.Release()
	takeAsync(t, openQ(t, dir), job("build", PriorityLane, 1, 100, false), nil)
	waitForWaiters(t, openQ(t, dir), 1)
	takeAsync(t, openQ(t, dir), job("typecheck", PriorityInteractive, 1, 100, false), nil)
	waitForWaiters(t, openQ(t, dir), 2)
	running, waiting, err := openQ(t, dir).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0].Class != "suite" {
		t.Errorf("running = %+v", running)
	}
	if len(waiting) != 2 || waiting[0].Class != "typecheck" || waiting[1].Class != "build" {
		t.Errorf("waiting order = %v", waiting)
	}
}

func TestLiveTicket(t *testing.T) {
	dir := t.TempDir()
	q := openQ(t, dir)
	h := mustHold(t, takeAsync(t, q, job("files", PriorityLane, 1, 100, false), nil))
	if !q.Live(h.Ticket.Seq, h.Ticket.PID) {
		t.Error("held ticket not live")
	}
	if q.Live(h.Ticket.Seq, h.Ticket.PID+1) {
		t.Error("ticket live under the wrong pid")
	}
	h.Release()
	if q.Live(h.Ticket.Seq, h.Ticket.PID) {
		t.Error("released ticket still live")
	}
}

func TestOpenFailsOnUnusableDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "queue")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(blocker); err == nil {
		t.Fatal("Open succeeded on a regular file")
	}
}
