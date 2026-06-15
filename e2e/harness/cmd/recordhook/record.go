//go:build harness_live

package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// record reads all of r (up to timeout) and appends it to the file at path.
// The timeout guards against a driven CLI (notably codex) holding the hook's
// stdin open, which would otherwise block the whole capture run forever.
func record(r io.Reader, path string, timeout time.Duration) error {
	type result struct {
		data []byte
		err  error
	}
	// Buffered (size 1) so this goroutine's send never blocks even after a
	// timeout return. On timeout the io.ReadAll goroutine is intentionally
	// ABANDONED — it stays blocked on a held-open stdin until the recordhook
	// process exits 0 (main reaps it). Acceptable for a short-lived hook helper.
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		ch <- result{data, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		// APPEND (not truncate): a single driven prompt fires several hooks
		// (PreToolUse, PostToolUse, Stop …), each rewritten to this recorder.
		// Appending one JSON payload per line keeps them ALL so capture can
		// select the right event afterward, instead of the last one clobbering
		// the rest (Plan B review B5).
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		// Normalize to a single line so the staging file is valid JSONL.
		line := append(bytesTrimNewlines(res.data), '\n')
		_, err = f.Write(line)
		return err
	case <-time.After(timeout):
		return fmt.Errorf("recordhook: stdin read timed out after %s", timeout)
	}
}

// bytesTrimNewlines removes embedded newlines so each recorded payload is one
// JSONL line (hook stdin is a single JSON object; this is belt-and-braces).
func bytesTrimNewlines(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != '\n' && c != '\r' {
			out = append(out, c)
		}
	}
	return out
}

// newBlockingReader returns a reader that blocks forever (for the timeout test)
// and a closer to release it.
func newBlockingReader() (io.Reader, func()) {
	pr, pw := io.Pipe()
	return pr, func() { _ = pw.Close() }
}
