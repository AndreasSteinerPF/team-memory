// Package git is a thin wrapper around the system git binary. TeamMemory shells
// out to git rather than using a Go git library so it inherits the user's exact
// git version, credentials, and transports (prd.md §16). The ledger is driven
// entirely through plumbing commands; the working tree and the repo's default
// index are never touched.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes git commands against a fixed repository directory.
type Runner struct {
	Dir string // repository directory; passed to git as "-C <Dir>"
}

// Run executes "git -C <Dir> <args...>" and returns stdout with the trailing
// newline trimmed. On a non-zero exit it returns an error that includes stderr.
func (r Runner) Run(args ...string) (string, error) {
	return r.exec(nil, nil, args...)
}

// RunInput is Run with data piped to the command's stdin (e.g. hash-object).
func (r Runner) RunInput(stdin []byte, args ...string) (string, error) {
	return r.exec(nil, stdin, args...)
}

// RunEnv is Run with extra environment variables appended to the inherited
// environment. Used to point index-writing commands at a private index file
// via GIT_INDEX_FILE so the repo's real index is never disturbed.
func (r Runner) RunEnv(env []string, args ...string) (string, error) {
	return r.exec(env, nil, args...)
}

// RefExists reports whether ref (e.g. "refs/heads/teammemory") resolves.
// Any failure is treated as "does not exist"; callers validate the repo first.
func (r Runner) RefExists(ref string) bool {
	err := exec.Command("git", "-C", r.Dir, "show-ref", "--verify", "--quiet", ref).Run()
	return err == nil
}

func (r Runner) exec(extraEnv []string, stdin []byte, args ...string) (string, error) {
	full := append([]string{"-C", r.Dir}, args...)
	cmd := exec.Command("git", full...)
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
