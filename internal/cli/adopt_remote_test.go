package cli_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func TestOpenEnvAdoptsFetchedRemoteLedgerBranch(t *testing.T) {
	repo := initRepo(t)
	tip := gitOut(t, repo, "rev-parse", "refs/heads/teammemory")
	gitOut(t, repo, "update-ref", "refs/remotes/origin/teammemory", tip)
	gitOut(t, repo, "update-ref", "-d", "refs/heads/teammemory")

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "list"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("list should adopt fetched ledger branch; rc=%d stderr=%s", code, errb.String())
	}
	if got := gitOut(t, repo, "rev-parse", "refs/heads/teammemory"); got != tip {
		t.Fatalf("local ledger ref = %s, want fetched remote tip %s", got, tip)
	}
	if !strings.Contains(out.String(), "No matching memories.") {
		t.Fatalf("list did not read the adopted ledger; stdout=%q stderr=%q", out.String(), errb.String())
	}
}

func TestOpenEnvDoesNotGuessBetweenMultipleFetchedLedgerBranches(t *testing.T) {
	repo := initRepo(t)
	tip := gitOut(t, repo, "rev-parse", "refs/heads/teammemory")
	gitOut(t, repo, "update-ref", "refs/remotes/origin/teammemory", tip)
	gitOut(t, repo, "update-ref", "refs/remotes/upstream/teammemory", tip)
	gitOut(t, repo, "update-ref", "-d", "refs/heads/teammemory")

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "list"}, strings.NewReader(""), &out, &errb)
	if code == 0 {
		t.Fatalf("list should fail when fetched ledger branch is ambiguous; stdout=%s", out.String())
	}
	if !strings.Contains(errb.String(), "multiple fetched remote ledger refs found") {
		t.Fatalf("stderr should explain ambiguity; got %q", errb.String())
	}
	if gitRefExists(t, repo, "refs/heads/teammemory") {
		t.Fatal("ambiguous fetched ledger refs should not create a local ledger branch")
	}
}

func TestInitAdoptsFetchedRemoteLedgerBranch(t *testing.T) {
	repo := initRepo(t)
	var proposeOut, proposeErr bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "propose", "decision", "--title", "Adopted memory", "--scope", "docs/**", "--guidance", "Keep the fetched ledger contents"}, strings.NewReader(""), &proposeOut, &proposeErr); code != 0 {
		t.Fatalf("propose failed (%d): %s", code, proposeErr.String())
	}
	tip := gitOut(t, repo, "rev-parse", "refs/heads/teammemory")
	gitOut(t, repo, "update-ref", "refs/remotes/origin/teammemory", tip)
	gitOut(t, repo, "update-ref", "-d", "refs/heads/teammemory")

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "init", "--no-push"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("init should adopt fetched ledger branch; rc=%d stderr=%s", code, errb.String())
	}
	if got := gitOut(t, repo, "rev-parse", "refs/heads/teammemory"); got != tip {
		t.Fatalf("local ledger ref = %s, want fetched remote tip %s", got, tip)
	}
	if !strings.Contains(out.String(), "Adopted fetched TeamMemory ledger") {
		t.Fatalf("init should report adoption; stdout=%q stderr=%q", out.String(), errb.String())
	}

	var listOut, listErr bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "list"}, strings.NewReader(""), &listOut, &listErr); code != 0 {
		t.Fatalf("list failed after adoption (%d): %s", code, listErr.String())
	}
	if !strings.Contains(listOut.String(), "Adopted memory") {
		t.Fatalf("adopted ledger contents not visible; stdout=%q stderr=%q", listOut.String(), listErr.String())
	}
}

func TestDoctorAdoptsFetchedRemoteLedgerBranch(t *testing.T) {
	repo := initRepo(t)
	tip := gitOut(t, repo, "rev-parse", "refs/heads/teammemory")
	gitOut(t, repo, "update-ref", "refs/remotes/origin/teammemory", tip)
	gitOut(t, repo, "update-ref", "-d", "refs/heads/teammemory")

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "doctor"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("doctor should adopt fetched ledger branch; rc=%d stdout=%s stderr=%s", code, out.String(), errb.String())
	}
	if got := gitOut(t, repo, "rev-parse", "refs/heads/teammemory"); got != tip {
		t.Fatalf("local ledger ref = %s, want fetched remote tip %s", got, tip)
	}
	if !strings.Contains(out.String(), "Ledger branch      initialized") {
		t.Fatalf("doctor should report initialized ledger; stdout=%q stderr=%q", out.String(), errb.String())
	}
}

func gitRefExists(t *testing.T, repo, ref string) bool {
	t.Helper()
	return exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", ref).Run() == nil
}

func gitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
