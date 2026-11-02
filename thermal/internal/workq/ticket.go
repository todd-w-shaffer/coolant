package workq

// Ticket is one job's place in the queue, stored as <seq>.json in the
// queue directory and rewritten in place (never renamed: the holder's
// flock lives on the inode).
type Ticket struct {
	Seq       int    `json:"seq"`
	PID       int    `json:"pid"`
	Lane      string `json:"lane,omitempty"`
	Priority  int    `json:"priority"`
	Class     string `json:"class"`
	Cores     int    `json:"cores"`
	MemMB     int    `json:"mem_mb"`
	Whole     bool   `json:"whole,omitempty"`
	State     string `json:"state"`
	Cwd       string `json:"cwd"`
	Repo      string `json:"repo,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	Cmd       string `json:"cmd"`
	QueuedAt  string `json:"queued_at"`
	StartedAt string `json:"started_at,omitempty"`
}
