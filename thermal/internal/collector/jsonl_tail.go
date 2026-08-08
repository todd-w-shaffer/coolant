package collector

import (
	"bufio"
	"io"
	"os"
)

// maxLineBytes caps the scanner's per-line buffer. Claude Code transcript lines
// can include base64-embedded tool results and long thinking signatures, well
// past bufio's 64KB default.
const maxLineBytes = 4 * 1024 * 1024

// LineHandler is invoked for each non-empty line read by TailFile.
// The line slice is reused by the scanner on the next iteration; handlers
// must not retain it past the call. Copy if persistence is needed.
type LineHandler func(line []byte)

// TailFile reads any lines appended to path since offset and invokes handler
// for each non-empty line. Returns the updated byte offset.
//
// Missing files, stat errors, and seek errors leave offset unchanged. When the
// file's size has shrunk below offset (truncation), TailFile resets to 0 and
// reads from the beginning. If done is non-nil and already closed, TailFile
// returns without invoking handler.
func TailFile(path string, offset int64, handler LineHandler, done <-chan struct{}) int64 {
	if isClosed(done) {
		return offset
	}

	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return offset
	}
	if info.Size() < offset {
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	for scanner.Scan() {
		if isClosed(done) {
			return offset
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		handler(line)
	}

	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return offset
	}
	return newOffset
}

func isClosed(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}
