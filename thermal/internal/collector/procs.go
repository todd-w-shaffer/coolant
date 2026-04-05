package collector

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// commToType maps a process name to a single-char type code.
// These feed into TypeToCategory in types.go for higher-level grouping.
var commToType = map[string]string{
	// Testing (V)
	"vitest":  "V",
	"jest":    "V",
	"mocha":   "V",
	"pytest":  "V",
	"phpunit": "V",

	// Build (T = compilers, B = bundlers/linters)
	"tsc":      "T",
	"webpack":  "B",
	"esbuild":  "B",
	"swc":      "B",
	"rollup":   "B",
	"vite":     "B",
	"turbo":    "B",
	"eslint":   "B",
	"prettier": "B",
	"rustc":    "B",
	"cargo":    "B",
	"go":       "B",
	"javac":    "B",
	"gcc":      "B",
	"clang":    "B",
	"make":     "B",

	// Runtime (N = node, P = python/etc)
	"node":    "N",
	"nodejs":  "N",
	"deno":    "N",
	"bun":     "N",
	"python":  "P",
	"python3": "P",
	"ruby":    "P",
	"java":    "P",
	"docker":  "P",
	"podman":  "P",

	// Search (G = grep-like, R = ripgrep, F = find)
	"grep":    "G",
	"rg":      "R",
	"ripgrep": "R",
	"find":    "F",
	"fd":      "F",
	"ag":      "G",

	// Shell (S = shell/scripting, C = utilities)
	"sh":   "S",
	"bash": "S",
	"zsh":  "S",
	"sed":  "S",
	"awk":  "S",
	"cat":  "C",
	"git":  "C",
	"curl": "C",
	"wget": "C",
}

// classifyComm returns the type code for a process name.
func classifyComm(comm string) string {
	comm = strings.ToLower(basename(comm))
	if code, ok := commToType[comm]; ok {
		return code
	}
	return "X"
}

type rawProc struct {
	pid  int
	ppid int
	cpu  float64
	rss  int64 // KB from ps
	comm string
}

// ProcCollector reuses pre-allocated maps across ticks to reduce GC pressure.
// Not safe for concurrent use — the fast loop calls Collect sequentially.
type ProcCollector struct {
	children map[int][]int   // ppid → [pid, ...]
	byPID    map[int]rawProc // pid → rawProc
	allProcs []rawProc       // reused backing slice
	roots    []int           // reused backing slice
	queue    []int           // BFS queue, reused
	visited  map[int]bool    // BFS visited set, reused
}

// clearMaps resets maps and slices for reuse without reallocating.
func (pc *ProcCollector) clearMaps() {
	for k := range pc.children {
		delete(pc.children, k)
	}
	for k := range pc.byPID {
		delete(pc.byPID, k)
	}
	pc.allProcs = pc.allProcs[:0]
	pc.roots = pc.roots[:0]
	if pc.visited == nil {
		pc.visited = make(map[int]bool, 256)
	} else {
		for k := range pc.visited {
			delete(pc.visited, k)
		}
	}
}

// Collect finds all Claude root sessions and builds their descendant trees.
// Reuses pre-allocated maps to avoid allocating every 150ms tick.
func (pc *ProcCollector) Collect(ctx context.Context) ([]SessionTree, []ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, config.ProcTimeout)
	defer cancel()

	// Get all processes in one call
	out, err := exec.CommandContext(ctx, "ps", "-Ao", "pid=,ppid=,pcpu=,rss=,comm=").Output()
	if err != nil {
		return nil, nil, err
	}

	pc.clearMaps()

	// Parse into raw process list and build parent→children map
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := parseProcessLine(line)
		if p.pid == 0 {
			continue
		}
		pc.allProcs = append(pc.allProcs, p)
		pc.children[p.ppid] = append(pc.children[p.ppid], p.pid)
	}

	// Index by PID for fast lookup
	for _, p := range pc.allProcs {
		pc.byPID[p.pid] = p
	}

	// Find Claude root processes
	for _, p := range pc.allProcs {
		name := strings.ToLower(basename(p.comm))
		if strings.Contains(name, "claude") {
			pc.roots = append(pc.roots, p.pid)
		}
	}

	// For each root, walk the descendant tree
	var sessions []SessionTree
	var flatProcs []ProcessInfo

	for _, rootPID := range pc.roots {
		root, ok := pc.byPID[rootPID]
		if !ok {
			continue
		}

		// BFS to find all descendants
		pc.queue = pc.queue[:0]
		pc.queue = append(pc.queue, rootPID)
		pc.visited[rootPID] = true
		var descendants []ProcessInfo

		for len(pc.queue) > 0 {
			pid := pc.queue[0]
			pc.queue = pc.queue[1:]

			for _, childPID := range pc.children[pid] {
				if pc.visited[childPID] {
					continue
				}
				pc.visited[childPID] = true
				pc.queue = append(pc.queue, childPID)

				if child, ok := pc.byPID[childPID]; ok {
					pi := ProcessInfo{
						PID:      child.pid,
						PPID:     child.ppid,
						CPUPct:   child.cpu,
						RSSBytes: child.rss * 1024, // KB → bytes
						Comm:     basename(child.comm),
						TypeCode: classifyComm(child.comm),
					}
					descendants = append(descendants, pi)
					flatProcs = append(flatProcs, pi)
				}
			}
		}

		sessions = append(sessions, SessionTree{
			RootPID:     rootPID,
			RootComm:    basename(root.comm),
			Descendants: descendants,
		})
	}

	return sessions, flatProcs, nil
}

// CollectProcs is a convenience wrapper for callers that don't need map reuse.
func CollectProcs(ctx context.Context) ([]SessionTree, []ProcessInfo, error) {
	pc := &ProcCollector{
		children: make(map[int][]int, 512),
		byPID:    make(map[int]rawProc, 512),
	}
	return pc.Collect(ctx)
}

// parseProcessLine parses one line of "ps -Ao pid=,ppid=,pcpu=,rss=,comm=" output.
func parseProcessLine(line string) rawProc {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return rawProc{}
	}
	pid, _ := strconv.Atoi(fields[0])
	ppid, _ := strconv.Atoi(fields[1])
	cpu, _ := strconv.ParseFloat(fields[2], 64)
	rss, _ := strconv.ParseInt(fields[3], 10, 64)
	// comm may contain spaces if it's a path; take everything after field 4
	comm := strings.Join(fields[4:], " ")
	return rawProc{pid: pid, ppid: ppid, cpu: cpu, rss: rss, comm: comm}
}

func basename(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
