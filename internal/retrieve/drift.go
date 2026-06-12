package retrieve

import (
	"strconv"
	"strings"
)

// gitRunner is the subset of *git.Runner that drift needs.
type gitRunner interface {
	Run(args ...string) (string, error)
}

// GitDrift computes anchor drift by shelling out to git in the code repo. It
// satisfies DriftSource. *git.Runner satisfies the embedded gitRunner.
type GitDrift struct{ Git gitRunner }

// Drift reports whether path exists at HEAD and how many commits have touched
// it since sinceCommit. A missing path short-circuits to (false, 0). An unknown
// anchor commit yields (true, -1): the file is there but the count is unknowable.
func (g GitDrift) Drift(path, sinceCommit string) (bool, int, error) {
	if _, err := g.Git.Run("cat-file", "-e", "HEAD:"+path); err != nil {
		return false, 0, nil // not present at HEAD
	}
	if sinceCommit == "" {
		return true, -1, nil
	}
	out, err := g.Git.Run("rev-list", "--count", sinceCommit+"..HEAD", "--", path)
	if err != nil {
		return true, -1, nil // unknown commit ⇒ count unknown, not a hard error
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return true, -1, nil
	}
	return true, n, nil
}
