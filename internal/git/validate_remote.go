package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ValidateRemote runs `git ls-remote <remote>` with the given timeout to verify
// that the remote is reachable. It does NOT push or write anything; it only
// confirms the URL/name resolves and that the caller has read access.
//
// A bare name (e.g. "origin") is resolved by git's own remote-name lookup. A
// URL/path is contacted directly. The repoDir argument is the working directory
// for the git call, so bare names resolve against that repo's configured remotes.
//
// Errors from ls-remote are returned verbatim (with the stderr appended) so
// callers can show the user exactly what git said.
func ValidateRemote(repoDir, remote string, timeout time.Duration) error {
	if remote == "" {
		return fmt.Errorf("validate-remote: empty remote")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "ls-remote", remote)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := strings.TrimSpace(stderr.String())
		if s == "" {
			return fmt.Errorf("ls-remote %s: %w", remote, err)
		}
		return fmt.Errorf("ls-remote %s: %w: %s", remote, err, s)
	}
	return nil
}
