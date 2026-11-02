package workq

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuf is a goroutine-safe writer for capturing door output.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func testSettings(t *testing.T, mode string) (Settings, *syncBuf, *syncBuf) {
	t.Helper()
	dir := t.TempDir()
	out, errb := &syncBuf{}, &syncBuf{}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	return Settings{
		QueueDir:    filepath.Join(dir, "queue"),
		RecordsPath: filepath.Join(dir, "runs.jsonl"),
		Mode:        mode,
		Budget:      budget8,
		Classes:     DefaultClassesForTest(),
		Env:         env,
		Stdout:      out,
		Stderr:      errb,
		SampleEvery: 20 * time.Millisecond,
		Poll:        10 * time.Millisecond,
	}, out, errb
}

// DefaultClassesForTest mirrors the shipped weights closely enough for
// admission to behave as it will in production.
func DefaultClassesForTest() map[string]Class {
	return map[string]Class{
		"suite":     {Cores: 8, MemoryMB: 3584, Whole: true},
		"files":     {Cores: 2, MemoryMB: 1536},
		"typecheck": {Cores: 1, MemoryMB: 1536},
	}
}

func onlyRecord(t *testing.T, s Settings) Record {
	t.Helper()
	recs := ReadRecent(s.RecordsPath, 1<<20)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	return recs[0]
}

func TestRunAnswersTheCommandsExitStatus(t *testing.T) {
	s, _, _ := testSettings(t, ModeObserve)
	if code := Run(context.Background(), s, []string{"--class", "typecheck", "--", "sh", "-c", "exit 3"}); code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	if r := onlyRecord(t, s); r.Exit != 3 {
		t.Errorf("recorded exit = %d", r.Exit)
	}
}

func TestRunAnswers127ForAMissingCommand(t *testing.T) {
	s, _, _ := testSettings(t, ModeObserve)
	if code := Run(context.Background(), s, []string{"--class", "typecheck", "--", "/nonexistent/cmd"}); code != 127 {
		t.Fatalf("exit = %d, want 127", code)
	}
}

func TestRunRecordsTheJob(t *testing.T) {
	s, _, _ := testSettings(t, ModeObserve)
	s.Env = append(s.Env, "COOLANT_LANE=nightly")
	busy := "i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done & sleep 0.3; wait"
	if code := Run(context.Background(), s, []string{"--class=files", "--", "sh", "-c", busy}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	r := onlyRecord(t, s)
	if r.Class != "files" || r.Lane != "nightly" || r.Interactive || r.Mode != ModeObserve {
		t.Errorf("identity = %+v", r)
	}
	if !strings.HasPrefix(r.Cmd, "sh -c 'i=0;") {
		t.Errorf("cmd = %q", r.Cmd)
	}
	if r.WallS < 0.25 || r.CPUS <= 0.05 {
		t.Errorf("wall %.2f / cpu %.2f: the grandchild's CPU must be counted", r.WallS, r.CPUS)
	}
	if r.PeakMemMB <= 0 || r.MemSamples == 0 {
		t.Errorf("peak mem %.1f MB over %d samples", r.PeakMemMB, r.MemSamples)
	}
	if r.WaitS != 0 || r.Cwd == "" || r.TS == "" || r.End == "" {
		t.Errorf("record = %+v", r)
	}
}

func TestRunObserveSetsTicketButNoGrant(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	Run(context.Background(), s, []string{"--class", "suite", "--", "sh", "-c", `echo "t=$COOLANT_TICKET g=$COOLANT_GRANTED_CORES"`})
	got := out.String()
	if !strings.Contains(got, "t=1:") || !strings.Contains(got, "g=\n") {
		t.Errorf("child saw %q", got)
	}
}

func TestRunEnforceHandsTheChildItsGrant(t *testing.T) {
	cases := map[string]string{"suite": "8", "files": "2"}
	for class, want := range cases {
		s, out, _ := testSettings(t, ModeEnforce)
		Run(context.Background(), s, []string{"--class", class, "--", "sh", "-c", `echo "g=$COOLANT_GRANTED_CORES"`})
		if got := strings.TrimSpace(out.String()); got != "g="+want {
			t.Errorf("%s: child saw %q, want g=%s", class, got, want)
		}
		if r := onlyRecord(t, s); r.GrantedCores != atoiOr(want) {
			t.Errorf("%s: granted_cores = %d", class, r.GrantedCores)
		}
	}
}

func atoiOr(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestRunWaitsAndPrintsWhatIsAhead(t *testing.T) {
	s, _, errb := testSettings(t, ModeEnforce)
	q := openQ(t, s.QueueDir)
	head, err := q.Take(context.Background(), Ticket{PID: 4242, Lane: "epic", Class: "suite", Whole: true, Cwd: "/w/other"}, budget8, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() { done <- Run(context.Background(), s, []string{"--class", "typecheck", "--", "true"}) }()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(errb.String(), "waiting") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	head.Release()
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d", code)
	}
	line := errb.String()
	for _, want := range []string{"1 ahead", "pid 4242", "epic", "suite", "/w/other"} {
		if !strings.Contains(line, want) {
			t.Errorf("wait output %q missing %q", line, want)
		}
	}
	if r := onlyRecord(t, s); r.WaitS <= 0 {
		t.Errorf("wait_s = %v, want > 0", r.WaitS)
	}
}

func TestRunNestedUnderALiveAncestorTicketPassesThrough(t *testing.T) {
	s, out, errb := testSettings(t, ModeEnforce)
	// Outer whole suite holds the whole budget; the inner suite would wait
	// forever if it took a ticket of its own.
	inner := os.Args[0] + " -test.run=^$"
	s.Env = append(s.Env,
		"WORKQ_DOOR_HELPER=1",
		"WORKQ_HELPER_QUEUE="+s.QueueDir,
		"WORKQ_HELPER_RECORDS="+s.RecordsPath,
	)
	done := make(chan int, 1)
	go func() {
		done <- Run(context.Background(), s, []string{"--class", "suite", "--", "sh", "-c", inner + ` -- --class suite -- sh -c 'echo inner-ran'`})
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("nested run deadlocked")
	}
	if !strings.Contains(out.String(), "inner-ran") {
		t.Fatalf("inner did not run: %q / %q", out.String(), errb.String())
	}
	recs := ReadRecent(s.RecordsPath, 1<<20)
	if len(recs) != 2 || !recs[0].Nested || recs[1].Nested || recs[0].Seq != recs[1].Seq {
		t.Fatalf("records = %+v", recs)
	}
}

func TestRunRefusesATicketWhoseHolderIsDead(t *testing.T) {
	s, _, errb := testSettings(t, ModeObserve)
	s.Env = append(s.Env, "COOLANT_TICKET=77:99999")
	if code := Run(context.Background(), s, []string{"--class", "typecheck", "--", "true"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "stale") {
		t.Errorf("stderr = %q, want a stale-ticket warning", errb.String())
	}
	if r := onlyRecord(t, s); r.Nested || r.Seq == 77 {
		t.Errorf("stale ticket passed: %+v", r)
	}
}

func TestRunRefusesALiveTicketOutsideItsProcessTree(t *testing.T) {
	s, _, errb := testSettings(t, ModeObserve)
	q := openQ(t, s.QueueDir)
	// Held by this very process: live, but not an ancestor of the caller.
	h, err := q.Take(context.Background(), Ticket{PID: os.Getpid(), Class: "suite", Whole: true}, budget8, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	s.Env = append(s.Env, "COOLANT_TICKET="+ticketEnv(h.Ticket.Seq, os.Getpid()))
	Run(context.Background(), s, []string{"--class", "typecheck", "--", "true"})
	if !strings.Contains(errb.String(), "stale") {
		t.Errorf("stderr = %q, want a stale-ticket warning", errb.String())
	}
	if r := onlyRecord(t, s); r.Nested {
		t.Error("a ticket from outside the caller's tree passed")
	}
}

func TestRunDegradesWhenTheQueueCannotBeUsed(t *testing.T) {
	s, _, errb := testSettings(t, ModeEnforce)
	if err := os.WriteFile(s.QueueDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run(context.Background(), s, []string{"--class", "suite", "--", "sh", "-c", "exit 5"}); code != 5 {
		t.Fatalf("exit = %d, want the command's 5", code)
	}
	if n := strings.Count(errb.String(), "coolant:"); n != 1 {
		t.Errorf("warnings = %d (%q), want exactly one", n, errb.String())
	}
	if r := onlyRecord(t, s); !r.Degraded {
		t.Error("record not marked degraded")
	}
}

func TestRunWarnsOnUnknownClassAndRuns(t *testing.T) {
	s, _, errb := testSettings(t, ModeObserve)
	if code := Run(context.Background(), s, []string{"--class", "mutation", "--", "true"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "mutation") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--class", "suite"},
		{"--class", "suite", "--"},
		{"--", "true"},
		{"--class"},
		{"--bogus", "x", "--", "true"},
	} {
		s, _, _ := testSettings(t, ModeObserve)
		if code := Run(context.Background(), s, args); code != ExitUsage {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestShellJoinQuotesWhatNeedsIt(t *testing.T) {
	got := ShellJoin([]string{"vitest", "run", "-t", "a b", "it's", ""})
	want := `vitest run -t 'a b' 'it'\''s' ''`
	if got != want {
		t.Errorf("ShellJoin = %s, want %s", got, want)
	}
}

func TestRunHonoursLeadingAssignmentsLikeEnv(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	code := Run(context.Background(), s, []string{"--class", "files", "--", "CI=1", "NODE_ENV=a b", "sh", "-c", `echo "$CI/$NODE_ENV"`})
	if code != 0 || strings.TrimSpace(out.String()) != "1/a b" {
		t.Fatalf("exit %d, child saw %q", code, out.String())
	}
}

func TestRunRejectsAssignmentsWithNoCommand(t *testing.T) {
	s, _, _ := testSettings(t, ModeObserve)
	if code := Run(context.Background(), s, []string{"--class", "files", "--", "CI=1"}); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestGitPlaceFindsRepoAndWorktreeWithoutGit(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "repo")
	wt := filepath.Join(root, "wt")
	gitdir := filepath.Join(main, ".git", "worktrees", "wt")
	for _, d := range []string{filepath.Join(main, "sub"), gitdir, filepath.Join(wt, "pkg")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, w := gitPlace(filepath.Join(main, "sub")); r != main || w != main {
		t.Errorf("main checkout: repo %q worktree %q", r, w)
	}
	if r, w := gitPlace(filepath.Join(wt, "pkg")); r != main || w != wt {
		t.Errorf("worktree: repo %q worktree %q, want %q %q", r, w, main, wt)
	}
	if r, w := gitPlace(root); r != "" || w != "" {
		t.Errorf("outside a repo: %q %q", r, w)
	}
}
