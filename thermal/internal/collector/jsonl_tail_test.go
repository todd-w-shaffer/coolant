package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTailFileReadsAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"+`{"a":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string
	off := TailFile(path, 0, func(line []byte) { got = append(got, string(line)) }, nil)

	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if got[0] != `{"a":1}` || got[1] != `{"a":2}` {
		t.Errorf("lines = %v", got)
	}
	if off == 0 {
		t.Errorf("offset did not advance: %d", off)
	}
}

func TestTailFileResumesFromOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	off := TailFile(path, 0, func(line []byte) {}, nil)

	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"+`{"a":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string
	TailFile(path, off, func(line []byte) { got = append(got, string(line)) }, nil)

	if len(got) != 1 {
		t.Fatalf("got %d lines after resume, want 1 (only the new line)", len(got))
	}
	if got[0] != `{"a":2}` {
		t.Errorf("got %q, want second line only", got[0])
	}
}

func TestTailFileResetsOnTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"+`{"a":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	off := TailFile(path, 0, func(line []byte) {}, nil)

	if err := os.WriteFile(path, []byte(`{"a":99}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string
	newOff := TailFile(path, off, func(line []byte) { got = append(got, string(line)) }, nil)

	if len(got) != 1 || got[0] != `{"a":99}` {
		t.Errorf("expected single line {\"a\":99} after truncation, got %v", got)
	}
	wantOff := int64(len(`{"a":99}` + "\n"))
	if newOff != wantOff {
		t.Errorf("offset after truncation = %d, want %d (proves reset to 0 + advance)", newOff, wantOff)
	}
}

func TestTailFileSkipsEmptyLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n\n"+`{"a":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var count int
	TailFile(path, 0, func(line []byte) { count++ }, nil)

	if count != 2 {
		t.Errorf("handler called %d times, want 2 (empty line should be skipped)", count)
	}
}

func TestTailFileMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.jsonl")

	called := false
	off := TailFile(path, 42, func(line []byte) { called = true }, nil)

	if called {
		t.Error("handler called for missing file")
	}
	if off != 42 {
		t.Errorf("offset = %d, want 42 (unchanged)", off)
	}
}

func TestTailFileDoneAborts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"+`{"a":2}`+"\n"+`{"a":3}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	close(done)

	var got []string
	TailFile(path, 0, func(line []byte) { got = append(got, string(line)) }, done)

	if len(got) != 0 {
		t.Errorf("got %d lines when done was already closed, want 0", len(got))
	}
}
