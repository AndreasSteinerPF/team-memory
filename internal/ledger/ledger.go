package ledger

import (
	"fmt"
	"os"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

const (
	memoriesDir     = "memories"
	observationsDir = "observations"
	policyFile      = "policy.yaml"
)

// Ledger is a handle to the orphan-branch ledger inside a git repository.
type Ledger struct {
	git    git.Runner
	branch string
}

// Open returns a ledger handle for branch within the git repository at repoDir.
// It verifies repoDir is a git repository but does not require the branch to
// already exist (call Init for that).
func Open(repoDir, branch string) (*Ledger, error) {
	g := git.Runner{Dir: repoDir}
	if _, err := g.Run("rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("ledger: %q is not a git repository: %w", repoDir, err)
	}
	return &Ledger{git: g, branch: branch}, nil
}

func (l *Ledger) ref() string { return "refs/heads/" + l.branch }

// Exists reports whether the ledger branch has been created.
func (l *Ledger) Exists() bool { return l.git.RefExists(l.ref()) }

// Init creates the orphan branch with an initial commit containing policy.yaml.
// It fails if the branch already exists.
func (l *Ledger) Init(policyYAML []byte) error {
	if l.Exists() {
		return fmt.Errorf("ledger: branch %q already exists", l.branch)
	}
	_, err := l.commitFiles("tm: initialize ledger",
		map[string][]byte{policyFile: policyYAML})
	return err
}

// commitFiles writes each path→content pair as a blob, layers them onto the
// current branch tree (if any) using a private index file, commits the result,
// and advances the branch ref. The working tree and the repo's default index
// are never touched, because every index operation runs against GIT_INDEX_FILE.
func (l *Ledger) commitFiles(message string, files map[string][]byte) (string, error) {
	idxFile, cleanup, err := tempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + idxFile}

	hasParent := l.Exists()
	parent := ""
	if hasParent {
		if parent, err = l.git.Run("rev-parse", l.ref()); err != nil {
			return "", err
		}
		// Seed the private index with the parent commit's tree.
		if _, err := l.git.RunEnv(env, "read-tree", parent); err != nil {
			return "", err
		}
	}

	for path, content := range files {
		sha, err := l.git.RunInput(content, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		if _, err := l.git.RunEnv(env, "update-index", "--add",
			"--cacheinfo", "100644,"+sha+","+path); err != nil {
			return "", err
		}
	}

	tree, err := l.git.RunEnv(env, "write-tree")
	if err != nil {
		return "", err
	}

	commitArgs := []string{"commit-tree", tree, "-m", message}
	if hasParent {
		commitArgs = append(commitArgs, "-p", parent)
	}
	commit, err := l.git.Run(commitArgs...)
	if err != nil {
		return "", err
	}

	if _, err := l.git.Run("update-ref", l.ref(), commit); err != nil {
		return "", err
	}
	return commit, nil
}

// tempIndex returns a path for a throwaway git index plus a cleanup func. git
// creates the index itself, so we hand it a path that does not yet exist.
func tempIndex() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "tm-index-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return "", nil, err
	}
	return name, func() { _ = os.Remove(name) }, nil
}
