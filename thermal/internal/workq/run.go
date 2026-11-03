package workq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// Modes — config owns them; these are the same constants.
const (
	ModeObserve = config.QueueModeObserve
	ModeEnforce = config.QueueModeEnforce
)

// Environment the door reads and sets.
const (
	EnvLane          = "COOLANT_LANE"
	EnvTicket        = "COOLANT_TICKET"
	EnvGrantedCores  = "COOLANT_GRANTED_CORES"
	defaultSampleInt = 200 * time.Millisecond
)

// Door exit statuses of its own. Every other status is the command's.
const (
	ExitUsage    = 64  // EX_USAGE: bad flags, no command
	ExitNotFound = 127 // the command could not be found
	ExitNoExec   = 126 // the command was found but could not start
)

// Class is one job class's weight, as config declares it.
type Class = config.QueueClass

// unknownClass is the weight of a class neither built in nor in config.
var unknownClass = Class{Cores: 1, MemoryMB: 512}

// Settings is everything Run needs from the outside world.
type Settings struct {
	QueueDir    string
	RecordsPath string
	Mode        string
	Budget      Budget
	Classes     map[string]Class
	// Env is the caller's environment; the child gets it plus the
	// ticket (and the grant in enforce mode).
	Env         []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	SampleEvery time.Duration
	Poll        time.Duration
}

// Run is `thermo run`: args are `--class <name> -- [NAME=value...] <cmd…>`.
// Leading NAME=value words set the command's environment, as env(1)
// does. It answers the command's exit status, or ExitUsage for a
// malformed call.
func Run(ctx context.Context, s Settings, args []string) int {
	if s.Stderr == nil {
		s.Stderr = os.Stderr
	}
	if s.Stdout == nil {
		s.Stdout = os.Stdout
	}
	if s.SampleEvery <= 0 {
		s.SampleEvery = defaultSampleInt
	}
	className, argv, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintf(s.Stderr, "coolant: %v\nusage: coolant run --class <name> -- <command...>\n", err)
		return ExitUsage
	}
	class, ok := s.Classes[className]
	if !ok {
		fmt.Fprintf(s.Stderr, "coolant: unknown class %q, weighing it as %d core / %d MB\n", className, unknownClass.Cores, unknownClass.MemoryMB)
		class = unknownClass
	}

	lane := envGet(s.Env, EnvLane)
	cwd, _ := os.Getwd()
	repo, worktree := gitPlace(cwd)
	t := Ticket{
		PID: os.Getpid(), Lane: lane, Priority: PriorityInteractive,
		Class: className, Cores: class.Cores, MemMB: class.MemoryMB, Whole: class.Whole,
		Cwd: cwd, Repo: repo, Worktree: worktree, Cmd: displayCmd(argv),
	}
	if lane != "" {
		t.Priority = PriorityLane
	}
	rec := Record{
		Mode: s.Mode, Class: className, Lane: lane, Interactive: lane == "",
		Repo: repo, Worktree: worktree, Cwd: cwd, Cmd: t.Cmd, PID: os.Getpid(),
		MemSampleMS: int(s.SampleEvery / time.Millisecond),
	}

	// Signals: a wait is abandoned on any of these; a running child is
	// sent TERM and HUP (INT and QUIT reach it from the terminal's
	// process group on their own).
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	waitCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go func() {
		select {
		case sig := <-sigs:
			cancel(signalCause{sig.(syscall.Signal)})
		case <-waitCtx.Done():
		}
	}()

	env, release, abandoned := admit(waitCtx, s, t, &rec)
	if abandoned != 0 {
		return abandoned
	}
	defer release()
	cancel(nil) // the wait is over; signals now go to the child
	return finish(s, &rec, launch(s, argv, env, sigs, &rec))
}

// signalCause is why a wait was abandoned.
type signalCause struct{ sig syscall.Signal }

func (c signalCause) Error() string { return c.sig.String() }

// admit settles how the job runs: passed through on a live ancestor's
// ticket, queued (in enforce, after its wait), or — when the queue
// cannot be used — degraded to running unqueued. It answers the child's
// environment and what to release when the job ends, or a non-zero
// exit status when the wait was abandoned.
func admit(ctx context.Context, s Settings, t Ticket, rec *Record) (env []string, release func(), abandoned int) {
	env, release = s.Env, func() {}
	degrade := func(err error) ([]string, func(), int) {
		fmt.Fprintf(s.Stderr, "coolant: queue at %s unusable (%v); running unqueued\n", s.QueueDir, err)
		rec.Degraded = true
		return env, release, 0
	}
	q, err := Open(s.QueueDir)
	if err != nil {
		return degrade(err)
	}
	if s.Poll > 0 {
		q.Poll = s.Poll
	}

	if tv := envGet(s.Env, EnvTicket); tv != "" {
		seq, pid, perr := parseTicketEnv(tv)
		if perr == nil && q.Live(seq, pid) && IsAncestor(pid) {
			q.Close()
			rec.Nested, rec.Seq = true, seq
			return env, release, 0
		}
		fmt.Fprintf(s.Stderr, "coolant: ignoring stale %s=%s (its holder is gone or outside this process tree); taking a turn\n", EnvTicket, tv)
	}

	enforce := s.Mode == ModeEnforce
	h, err := q.Take(ctx, t, s.Budget, enforce, func(ahead []Ticket) {
		fmt.Fprintln(s.Stderr, WaitLine(ahead))
	})
	if err != nil {
		q.Close()
		if ctx.Err() != nil {
			if c, ok := context.Cause(ctx).(signalCause); ok {
				return nil, nil, 128 + int(c.sig)
			}
			return nil, nil, 128 + int(syscall.SIGINT)
		}
		return degrade(err)
	}
	rec.Seq = h.Ticket.Seq
	rec.WaitS = round2(h.Waited.Seconds())
	rec.WouldWait = h.WouldWait
	rec.OthersRunning, rec.OthersCores, rec.OthersMemMB = h.OthersRunning, h.OthersCores, h.OthersMemMB
	env = envSet(env, EnvTicket, ticketEnv(h.Ticket.Seq, os.Getpid()))
	if enforce {
		rec.GrantedCores = h.Ticket.Need(s.Budget).Cores
		env = envSet(env, EnvGrantedCores, strconv.Itoa(rec.GrantedCores))
	}
	return env, func() { h.Release(); q.Close() }, 0
}

// launch runs argv as a child, sampling its tree's memory, and fills
// the timing fields of rec. It answers the child's exit status.
func launch(s Settings, argv, env []string, sigs chan os.Signal, rec *Record) int {
	rec.LoadStart = round2(LoadAverage())
	for len(argv) > 0 && isAssignment(argv[0]) {
		name, val, _ := strings.Cut(argv[0], "=")
		env = envSet(env, name, val)
		argv = argv[1:]
	}
	start := time.Now()
	rec.TS = start.UTC().Format(time.RFC3339)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin = s.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	if err := cmd.Start(); err != nil {
		rec.End = time.Now().UTC().Format(time.RFC3339)
		fmt.Fprintf(s.Stderr, "coolant: %v\n", err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return ExitNotFound
		}
		return ExitNoExec
	}
	pid := cmd.Process.Pid

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var peak uint64
	var samples int
	wg.Add(2)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(s.SampleEvery)
		defer tick.Stop()
		for {
			if b := TreeFootprint(pid); b > peak {
				peak = b
			}
			samples++
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case sig := <-sigs:
				if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
					_ = cmd.Process.Signal(sig)
				}
			}
		}
	}()
	_ = cmd.Wait()
	close(stop)
	wg.Wait()

	end := time.Now()
	rec.End = end.UTC().Format(time.RFC3339)
	rec.WallS = round2(end.Sub(start).Seconds())
	rec.LoadEnd = round2(LoadAverage())
	ps := cmd.ProcessState
	if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
		rec.CPUS = round2(tvSeconds(ru.Utime) + tvSeconds(ru.Stime))
		if mx := uint64(ru.Maxrss); mx > peak { // bytes on darwin
			peak = mx
		}
	}
	rec.PeakMemMB = round2(float64(peak) / (1 << 20))
	rec.MemSamples = samples
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

// finish writes the record (best effort: a records file that cannot be
// written never fails the job) and answers code.
func finish(s Settings, rec *Record, code int) int {
	rec.Exit = code
	if s.RecordsPath != "" {
		if err := AppendRecord(s.RecordsPath, *rec); err != nil {
			fmt.Fprintf(s.Stderr, "coolant: could not record the run in %s (%v)\n", s.RecordsPath, err)
		}
	}
	return code
}

func parseRunArgs(args []string) (class string, argv []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			argv = args[i+1:]
			i = len(args)
		case a == "--class":
			if i+1 >= len(args) {
				return "", nil, errors.New("--class needs a value")
			}
			class = args[i+1]
			i++
		case strings.HasPrefix(a, "--class="):
			class = strings.TrimPrefix(a, "--class=")
		default:
			return "", nil, fmt.Errorf("unexpected argument %q before --", a)
		}
	}
	if class == "" {
		return "", nil, errors.New("--class is required")
	}
	cmd := argv
	for len(cmd) > 0 && isAssignment(cmd[0]) {
		cmd = cmd[1:]
	}
	if len(cmd) == 0 {
		return "", nil, errors.New("no command after --")
	}
	return class, argv, nil
}

// displayCmd is argv as tickets, `coolant queue` and records show it:
// shell-quoted, with leading assignment values redacted (NAME=…), since
// those are where a token passed to one command would otherwise be kept.
func displayCmd(argv []string) string {
	shown := make([]string, len(argv))
	copy(shown, argv)
	for i := 0; i < len(shown) && isAssignment(shown[i]); i++ {
		name, _, _ := strings.Cut(shown[i], "=")
		shown[i] = name + "=…"
	}
	return ShellJoin(shown)
}

// isAssignment reports whether w is a NAME=value word.
func isAssignment(w string) bool {
	name, _, ok := strings.Cut(w, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func ticketEnv(seq, pid int) string { return fmt.Sprintf("%d:%d", seq, pid) }

func parseTicketEnv(v string) (seq, pid int, err error) {
	a, b, ok := strings.Cut(v, ":")
	if !ok {
		return 0, 0, errors.New("malformed ticket")
	}
	if seq, err = strconv.Atoi(a); err != nil {
		return 0, 0, err
	}
	if pid, err = strconv.Atoi(b); err != nil {
		return 0, 0, err
	}
	return seq, pid, nil
}

func envGet(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}

func envSet(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return append(out, key+"="+val)
}

// gitPlace answers the repository (the main checkout, shared by every
// worktree) and the worktree a directory is in, or empty strings. It
// reads .git from disk rather than forking git: a .git directory marks
// a main checkout; a .git file names a worktree's gitdir, whose
// commondir names the repository's.
func gitPlace(dir string) (repo, worktree string) {
	for d := dir; ; {
		dotgit := filepath.Join(d, ".git")
		if fi, err := os.Stat(dotgit); err == nil {
			if fi.IsDir() {
				return d, d
			}
			b, err := os.ReadFile(dotgit)
			if err != nil {
				return "", ""
			}
			gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
			if !ok {
				return "", ""
			}
			gitdir = resolveFrom(d, gitdir)
			common := gitdir
			if c, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
				common = resolveFrom(gitdir, strings.TrimSpace(string(c)))
			}
			if filepath.Base(common) == ".git" {
				return filepath.Dir(common), d
			}
			return common, d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", ""
		}
		d = parent
	}
}

func resolveFrom(base, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

// ShellJoin renders argv as one shell-readable line, single-quoting any
// word that is empty or holds a character the shell would interpret.
func ShellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.IndexFunc(a, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:,+@%…", r))
		}) < 0 {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

func tvSeconds(tv syscall.Timeval) float64 {
	return float64(tv.Sec) + float64(tv.Usec)/1e6
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
