package ledger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/recordid"
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

// AppendMemory assigns a ULID if none is set, stamps CreatedAt if zero,
// serializes the memory, and commits it as memories/<id>.yaml. It returns the
// memory's ID. The ledger branch must already exist (call Init first).
func (l *Ledger) AppendMemory(m model.Memory) (string, error) {
	if !l.Exists() {
		return "", fmt.Errorf("ledger: branch %q does not exist; run Init first", l.branch)
	}
	if m.ID == "" {
		m.ID = recordid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	data, err := marshalMemory(m)
	if err != nil {
		return "", err
	}
	path := memoriesDir + "/" + m.ID + ".yaml"
	if _, err := l.commitFiles("tm: add memory "+m.ID,
		map[string][]byte{path: data}); err != nil {
		return "", err
	}
	return m.ID, nil
}

// AppendObservation is the observation analogue of AppendMemory.
func (l *Ledger) AppendObservation(o model.Observation) (string, error) {
	if !l.Exists() {
		return "", fmt.Errorf("ledger: branch %q does not exist; run Init first", l.branch)
	}
	if o.ID == "" {
		o.ID = recordid.New()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	data, err := marshalObservation(o)
	if err != nil {
		return "", err
	}
	path := observationsDir + "/" + o.ID + ".yaml"
	if _, err := l.commitFiles("tm: add observation "+o.ID,
		map[string][]byte{path: data}); err != nil {
		return "", err
	}
	return o.ID, nil
}

// Memories returns every memory record on the branch.
func (l *Ledger) Memories() ([]model.Memory, error) {
	if !l.Exists() {
		return nil, nil
	}
	files, err := l.listFiles(memoriesDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.Memory, 0, len(files))
	for _, f := range files {
		data, err := l.readFile(f)
		if err != nil {
			return nil, err
		}
		m, err := unmarshalMemory(data)
		if err != nil {
			return nil, fmt.Errorf("ledger: parse %s: %w", f, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// Observations returns every observation record on the branch.
func (l *Ledger) Observations() ([]model.Observation, error) {
	if !l.Exists() {
		return nil, nil
	}
	files, err := l.listFiles(observationsDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.Observation, 0, len(files))
	for _, f := range files {
		data, err := l.readFile(f)
		if err != nil {
			return nil, err
		}
		o, err := unmarshalObservation(data)
		if err != nil {
			return nil, fmt.Errorf("ledger: parse %s: %w", f, err)
		}
		out = append(out, o)
	}
	return out, nil
}

// Policy returns the raw bytes of policy.yaml from the branch.
func (l *Ledger) Policy() ([]byte, error) {
	if !l.Exists() {
		return nil, fmt.Errorf("ledger: branch %q does not exist", l.branch)
	}
	return l.readFile(policyFile)
}

// listFiles returns the paths of every blob under dir on the branch.
func (l *Ledger) listFiles(dir string) ([]string, error) {
	out, err := l.git.Run("ls-tree", "-r", "--name-only", l.ref(), dir+"/")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// readFile returns the content of a single path on the branch.
func (l *Ledger) readFile(path string) ([]byte, error) {
	out, err := l.git.Run("cat-file", "-p", l.ref()+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
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
