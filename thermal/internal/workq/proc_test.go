package workq

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestIsAncestorFindsOwnParent(t *testing.T) {
	if !IsAncestor(os.Getppid()) {
		t.Fatalf("parent %d not seen as an ancestor", os.Getppid())
	}
}

func TestIsAncestorRejectsSelfChildAndStranger(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if IsAncestor(cmd.Process.Pid) {
		t.Error("a child is not an ancestor")
	}
	if IsAncestor(0) || IsAncestor(-1) {
		t.Error("invalid pids are not ancestors")
	}
}

func TestTreeFootprintSumsDescendants(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 3 & sleep 3 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	time.Sleep(300 * time.Millisecond)
	alone := TreeFootprint(cmd.Process.Pid + 1_000_000) // no such process
	tree := TreeFootprint(cmd.Process.Pid)
	if alone != 0 {
		t.Errorf("footprint of a missing pid = %d, want 0", alone)
	}
	// sh plus two sleeps: at least three processes' worth of footprint.
	if tree < 3*256*1024 {
		t.Errorf("tree footprint = %d bytes, want the shell and both sleeps counted", tree)
	}
}

func TestLoadAverageReads(t *testing.T) {
	if l := LoadAverage(); l < 0 {
		t.Fatalf("load = %v", l)
	}
}
