package collector

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// commToType maps a process name to a single-char type code.
var commToType = map[string]string{
	"node":    "N",
	"nodejs":  "N",
	"vitest":  "V",
	"jest":    "V",
	"tsc":     "T",
	"python":  "P",
	"python3": "P",
	"grep":    "G",
	"rg":      "R",
	"ripgrep": "R",
	"find":    "F",
	"sed":     "S",
	"sh":      "S",
	"bash":    "S",
	"zsh":     "S",
	"cat":     "C",
}

// classifyComm returns the type code for a process name.
func classifyComm(comm string) string {
	// Strip path to get basename
	if idx := strings.LastIndex(comm, "/"); idx >= 0 {
		comm = comm[idx+1:]
	}
	comm = strings.ToLower(comm)
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

// CollectProcs finds all Claude root sessions and builds their descendant trees.
func CollectProcs(ctx context.Context) ([]SessionTree, []ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Get all processes in one call
	out, err := exec.CommandContext(ctx, "ps", "-Ao", "pid=,ppid=,pcpu=,rss=,comm=").Output()
	if err != nil {
		return nil, nil, err
	}

	// Parse into raw process list and build parent→children map
	var allProcs []rawProc
	children := make(map[int][]int)  // ppid → [pid, ...]

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := parseProcessLine(line)
		if p.pid == 0 {
			continue
		}
		allProcs = append(allProcs, p)
		children[p.ppid] = append(children[p.ppid], p.pid)
	}

	// Index by PID for fast lookup
	byPID := make(map[int]rawProc, len(allProcs))
	for _, p := range allProcs {
		byPID[p.pid] = p
	}

	// Find Claude root processes
	var roots []int
	for _, p := range allProcs {
		name := strings.ToLower(p.comm)
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if strings.Contains(name, "claude") {
			roots = append(roots, p.pid)
		}
	}

	// For each root, walk the descendant tree
	var sessions []SessionTree
	var flatProcs []ProcessInfo
	visited := make(map[int]bool)

	for _, rootPID := range roots {
		root, ok := byPID[rootPID]
		if !ok {
			continue
		}

		// BFS to find all descendants
		queue := []int{rootPID}
		visited[rootPID] = true
		var descendants []ProcessInfo

		for len(queue) > 0 {
			pid := queue[0]
			queue = queue[1:]

			for _, childPID := range children[pid] {
				if visited[childPID] {
					continue
				}
				visited[childPID] = true
				queue = append(queue, childPID)

				if child, ok := byPID[childPID]; ok {
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
