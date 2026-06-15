//go:build harness_live

package harness_e2e

// LiveDriver knows how to drive one real CLI non-interactively.
type LiveDriver interface {
	// Command returns the binary and args to run the given prompt non-interactively.
	Command(prompt string) (bin string, args []string)
	// RecordHookCommand returns the shell command string that the installed hook
	// config should be rewritten to, so a fired hook runs the recordhook helper.
	// recordhookBin is the absolute path to the built helper.
	RecordHookCommand(recordhookBin string) string
}

var drivers = map[string]LiveDriver{}

func registerDriver(name string, d LiveDriver) { drivers[name] = d }

// GetDriver returns the live driver for a harness.
func GetDriver(name string) (LiveDriver, bool) { d, ok := drivers[name]; return d, ok }

func init() {
	registerDriver("claude", simpleDriver{
		bin: "claude", flags: []string{"--dangerously-skip-permissions"}, promptViaFlag: "-p",
	})
	registerDriver("codex", codexDriver{})
	registerDriver("copilot", simpleDriver{
		bin: "copilot", flags: []string{"--allow-all-tools"}, promptViaFlag: "-p",
	})
	registerDriver("gemini", simpleDriver{
		bin: "gemini", flags: []string{"--yolo"}, promptViaFlag: "-p",
	})
	// cursor-agent: headless CLI installed as `agent` (verified firing hooks,
	// cursor 2026.06.12). Prompt is a TRAILING positional; -p/--print = headless
	// with full tool access, --force = auto-approve, --trust = trust workspace.
	// NOTE: headless agent does NOT fire the stop/beforeSubmitPrompt hooks, so
	// cursor's nudge/prompt fixtures stay authored (see Task 7).
	registerDriver("cursor", simpleDriver{
		bin: "agent", flags: []string{"-p", "--force", "--trust"},
	})
}

// simpleDriver covers CLIs that take the prompt either as a trailing arg or
// after a -p flag.
type simpleDriver struct {
	bin           string
	flags         []string
	promptViaFlag string // if set, prompt follows this flag; else prompt is appended
}

func (d simpleDriver) Command(prompt string) (string, []string) {
	args := append([]string{}, d.flags...)
	if d.promptViaFlag != "" {
		args = append(args, d.promptViaFlag, prompt)
	} else {
		args = append(args, prompt)
	}
	return d.bin, args
}
func (simpleDriver) RecordHookCommand(bin string) string { return bin }

// codexDriver uses `codex exec` with the bypass flags and a trailing prompt.
type codexDriver struct{}

func (codexDriver) Command(prompt string) (string, []string) {
	return "codex", []string{"exec",
		"--dangerously-bypass-approvals-and-sandbox",
		"--dangerously-bypass-hook-trust", prompt}
}
func (codexDriver) RecordHookCommand(bin string) string { return bin }
