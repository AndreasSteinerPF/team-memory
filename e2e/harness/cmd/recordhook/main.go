//go:build harness_live

package main

import (
	"fmt"
	"os"
	"time"
)

// recordhook is a test-only hook command used during E2E capture. It records the
// hook stdin to $TM_RECORD_FILE and exits 0 so the driven CLI proceeds. It is
// NOT part of the shipped tm binary (built only under the harness_live tag).
func main() {
	dst := os.Getenv("TM_RECORD_FILE")
	if dst == "" {
		fmt.Fprintln(os.Stderr, "recordhook: TM_RECORD_FILE not set")
		os.Exit(0) // never block the driven CLI
	}
	if err := record(os.Stdin, dst, 4*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "recordhook:", err)
	}
	os.Exit(0)
}
