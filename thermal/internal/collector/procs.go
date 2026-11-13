package collector

import (
	"strings"
	"time"
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
	"javac":    "B",
	"gcc":      "B",
	"clang":    "B",
	"make":     "B",

	// Go runtime (GO)
	"go": "GO",

	// Rust runtime (RS)
	"cargo": "RS",
	"rustc": "RS",

	// Swift runtime (SW)
	"swift":          "SW",
	"swiftc":         "SW",
	"swift-frontend": "SW",
	"swift-build":    "SW",
	"sourcekit-lsp":  "SW",
	"xcodebuild":     "SW",
	"xctest":         "SW",

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
	comm string
}

// ProcCollector reuses pre-allocated maps across ticks to reduce GC pressure.
// Not safe for concurrent use — the fast loop calls Collect sequentially.
type ProcCollector struct {
	children          map[int][]int   // ppid → [pid, ...]
	byPID             map[int]rawProc // pid → rawProc
	allProcs          []rawProc       // reused backing slice
	roots             []int           // reused backing slice
	queue             []int           // BFS queue, reused
	visited           map[int]bool    // BFS visited set, reused
	lister            *procLister     // kernel process reader, created on first Collect
	DesktopRunning    bool            // true when Claude Desktop main process is detected
	ChromeHostRunning bool            // true when chrome-native-host (browser extension bridge) is detected
}

// clearMaps resets maps and slices for reuse without reallocating.
func (pc *ProcCollector) clearMaps() {
	clear(pc.children)
	clear(pc.byPID)
	pc.allProcs = pc.allProcs[:0]
	pc.roots = pc.roots[:0]
	if pc.visited == nil {
		pc.visited = make(map[int]bool, 256)
	} else {
		clear(pc.visited)
	}
}

// Collect finds all Claude root sessions and builds their descendant trees
// from a direct kernel read (procLister), reusing pre-allocated maps.
func (pc *ProcCollector) Collect() ([]SessionTree, []ProcessInfo) {
	if pc.lister == nil {
		pc.lister = newProcLister(&sysProcs{}, time.Now)
	}
	return pc.buildTrees(pc.lister.list())
}

// buildTrees builds session trees from a process list.
// Separated from Collect so it can be tested with synthetic input.
func (pc *ProcCollector) buildTrees(procs []rawProc) ([]SessionTree, []ProcessInfo) {
	pc.clearMaps()

	// Build the parent→children map
	for _, p := range procs {
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

	// Find Claude CLI root processes, excluding Desktop app and browser bridge
	pc.DesktopRunning = false
	pc.ChromeHostRunning = false
	for _, p := range pc.allProcs {
		if strings.Contains(p.comm, "Claude.app") {
			if strings.Contains(p.comm, "chrome-native-host") {
				pc.ChromeHostRunning = true
			} else {
				pc.DesktopRunning = true
			}
			continue
		}
		name := strings.ToLower(basename(p.comm))
		if strings.HasPrefix(name, "claude") {
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

	return sessions, flatProcs
}

func basename(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
