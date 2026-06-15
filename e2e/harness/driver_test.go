//go:build harness_live

package harness_e2e

import (
	"strings"
	"testing"
)

func TestDriverArgvContainsPrompt(t *testing.T) {
	for _, name := range []string{"claude", "codex", "copilot", "gemini"} {
		drv, ok := GetDriver(name)
		if !ok {
			t.Fatalf("no driver for %s", name)
		}
		bin, args := drv.Command("do the thing")
		if bin == "" {
			t.Errorf("%s: empty binary", name)
		}
		if !strings.Contains(strings.Join(args, " "), "do the thing") {
			t.Errorf("%s: argv missing prompt: %v", name, args)
		}
	}
}
