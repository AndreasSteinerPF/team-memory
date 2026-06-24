package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func runNudge(t *testing.T, repo, stdin string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "--hook"}, strings.NewReader(stdin), &out, &errb)
	return out.String(), code
}

func TestNudgeHookEmitsAfterFailPass(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	out, code := runNudge(t, repo, `{"session_id":"s1"}`)
	if code != 0 {
		t.Fatalf("nudge hook exit = %d", code)
	}
	if !strings.Contains(out, "tm_propose") || !strings.Contains(out, "failed_attempt") {
		t.Errorf("expected a propose nudge, got: %q", out)
	}
}

func TestNudgeHookSilentWithNoSignal(t *testing.T) {
	repo := initRepo(t)
	out, code := runNudge(t, repo, `{"session_id":"fresh"}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected silence on a fresh session, got: %q", out)
	}
}

func TestNudgeHookDoesNotRecordFailedRenderedDelivery(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	var errb bytes.Buffer
	code := cli.Run(
		[]string{"--repo", repo, "nudge", "--hook", "--harness", "gemini"},
		strings.NewReader(`{"session_id":"s1"}`),
		failingWriter{},
		&errb,
	)
	if code == 0 {
		t.Fatal("expected rendered delivery failure")
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j struct {
		Fired []struct {
			PendingDelivery bool      `json:"pending_delivery"`
			DeliveredAt     time.Time `json:"delivered_at"`
		} `json:"fired"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Fired) != 1 || !j.Fired[0].PendingDelivery || !j.Fired[0].DeliveredAt.IsZero() {
		t.Fatalf("failed rendered delivery must remain a retryable attempt: %+v", j.Fired)
	}
}

func TestNudgeHookSuccessfulRetryClearsDirectPendingDelivery(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	var errb bytes.Buffer
	args := []string{"--repo", repo, "nudge", "--hook", "--harness", "gemini"}
	if code := cli.Run(args, strings.NewReader(`{"session_id":"s1"}`), failingWriter{}, &errb); code == 0 {
		t.Fatal("expected initial render failure")
	}
	var retryOut bytes.Buffer
	if code := cli.Run(args, strings.NewReader(`{"session_id":"s1"}`), &retryOut, &errb); code != 0 {
		t.Fatalf("retry exit %d: %s", code, errb.String())
	}

	var reportOut bytes.Buffer
	if code := cli.Run(
		[]string{"--repo", repo, "nudge", "report", "--session", "s1", "--json"},
		strings.NewReader(""),
		&reportOut,
		&errb,
	); code != 0 {
		t.Fatalf("report exit %d: %s", code, errb.String())
	}
	var got struct {
		Fired   int `json:"fired"`
		Pending int `json:"pending"`
	}
	if err := json.Unmarshal(reportOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Fired != 1 || got.Pending != 0 {
		t.Fatalf("retry report mismatch: %+v", got)
	}
}

// TestNudgeHookQueuesPendingOnClaude pins the Stop→UserPromptSubmit re-delivery
// path required when the harness's Stop hook does not surface stdout to the
// agent (Claude Code; contested ledger memory 01KV84H0XQTPVWVNR65PG1TD2A). The
// nudge text must land in journal.Pending so the next prompt-signal hook can
// re-inject it via additionalContext.
func TestNudgeHookQueuesPendingOnClaude(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	if _, code := runNudge(t, repo, `{"session_id":"s1"}`); code != 0 {
		t.Fatalf("nudge exit %d", code)
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j struct {
		Pending []string `json:"pending"`
		Fired   []struct {
			Delivery  string `json:"delivery"`
			TextBytes int    `json:"text_bytes"`
		} `json:"fired"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Pending) != 1 || !strings.Contains(j.Pending[0], "tm_propose") {
		t.Fatalf("expected one tm_propose nudge queued in Pending, got: %v", j.Pending)
	}
	if len(j.Fired) != 1 || j.Fired[0].Delivery != "queued" || j.Fired[0].TextBytes == 0 {
		t.Fatalf("expected queued fired metadata, got: %+v", j.Fired)
	}
}

func TestNudgeHookQueuesPendingOnCodex(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "--hook", "--harness", "codex"}, strings.NewReader(`{"session_id":"s1"}`), &out, &errb)
	if code != 0 {
		t.Fatalf("codex nudge exit %d: %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("codex Stop nudge should stay silent and queue for prompt drain; got %q", out.String())
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j struct {
		Pending []string `json:"pending"`
		Fired   []struct {
			DrainedTurn int `json:"drained_turn"`
		} `json:"fired"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Pending) != 1 || !strings.Contains(j.Pending[0], "tm_propose") {
		t.Fatalf("expected one tm_propose nudge queued in Pending, got: %v", j.Pending)
	}

	var promptOut bytes.Buffer
	code = cli.Run([]string{"--repo", repo, "signal", "--hook", "--prompt", "--harness", "codex"}, strings.NewReader(`{"session_id":"s1"}`), &promptOut, &errb)
	if code != 0 {
		t.Fatalf("codex prompt signal exit %d: %s", code, errb.String())
	}
	if !strings.Contains(promptOut.String(), `"hookEventName":"UserPromptSubmit"`) ||
		!strings.Contains(promptOut.String(), `"additionalContext"`) ||
		!strings.Contains(promptOut.String(), "tm_propose") {
		t.Fatalf("expected codex prompt drain via additionalContext, got %q", promptOut.String())
	}
}

// TestPromptSignalDrainsPendingViaAdditionalContext pins the surfacing path on
// Claude: a queued nudge from a prior Stop event must be re-emitted on the
// next UserPromptSubmit inside hookSpecificOutput.additionalContext (the
// channel verified to reach the agent — Stop stdout doesn't), and the journal
// must clear Pending so the same nudge isn't re-delivered on subsequent
// prompts.
func TestPromptSignalDrainsPendingViaAdditionalContext(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)
	if _, code := runNudge(t, repo, `{"session_id":"s1"}`); code != 0 {
		t.Fatalf("nudge exit %d", code)
	}

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "signal", "--hook", "--prompt"}, strings.NewReader(`{"session_id":"s1"}`), &out, &errb)
	if code != 0 {
		t.Fatalf("prompt signal exit %d: %s", code, errb.String())
	}

	body := out.String()
	if !strings.Contains(body, "hookSpecificOutput") {
		t.Errorf("expected hookSpecificOutput envelope, got: %q", body)
	}
	if !strings.Contains(body, "additionalContext") {
		t.Errorf("expected additionalContext field, got: %q", body)
	}
	if !strings.Contains(body, "tm_propose") {
		t.Errorf("expected nudge text re-injected via additionalContext, got: %q", body)
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j struct {
		Pending []string `json:"pending"`
		Fired   []struct {
			DrainedTurn int `json:"drained_turn"`
		} `json:"fired"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Pending) != 0 {
		t.Errorf("Pending must be cleared after drain, got: %v", j.Pending)
	}
	if len(j.Fired) != 1 || j.Fired[0].DrainedTurn == 0 {
		t.Fatalf("expected queued nudge to be marked drained, got: %+v", j.Fired)
	}

	// A second prompt with no new Stop nudge must NOT re-emit (idempotent drain).
	var out2 bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "signal", "--hook", "--prompt"}, strings.NewReader(`{"session_id":"s1"}`), &out2, &errb); code != 0 {
		t.Fatalf("second prompt signal exit %d", code)
	}
	if strings.TrimSpace(out2.String()) != "" {
		t.Errorf("second prompt should emit nothing, got: %q", out2.String())
	}
}

func TestPromptSignalRetainsPendingWhenRenderFails(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)
	if _, code := runNudge(t, repo, `{"session_id":"s1"}`); code != 0 {
		t.Fatalf("nudge exit %d", code)
	}

	var errb bytes.Buffer
	code := cli.Run(
		[]string{"--repo", repo, "signal", "--hook", "--prompt"},
		strings.NewReader(`{"session_id":"s1"}`),
		failingWriter{},
		&errb,
	)
	if code == 0 {
		t.Fatal("expected render failure")
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j struct {
		Pending []string `json:"pending"`
		Fired   []struct {
			DrainedTurn int `json:"drained_turn"`
		} `json:"fired"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Pending) != 1 {
		t.Fatalf("pending nudge lost after render failure: %+v", j)
	}
	if len(j.Fired) != 1 || j.Fired[0].DrainedTurn != 0 {
		t.Fatalf("failed render must not mark nudge drained: %+v", j.Fired)
	}
}

func TestNudgeReportPrintsSummary(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)
	if _, code := runNudge(t, repo, `{"session_id":"s1"}`); code != 0 {
		t.Fatalf("nudge exit %d", code)
	}

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "report"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errb.String())
	}
	body := out.String()
	for _, want := range []string{"Nudge report", "Sessions: 1", "fired: 1", "queued: 1", "pending: 1", "approx context bytes:", "Follow-through:"} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q:\n%s", want, body)
		}
	}
}

func TestNudgeReportJSON(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)
	if _, code := runNudge(t, repo, `{"session_id":"s1"}`); code != 0 {
		t.Fatalf("nudge exit %d", code)
	}

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "report", "--json"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errb.String())
	}
	var got struct {
		Sessions           int `json:"sessions"`
		Fired              int `json:"fired"`
		Queued             int `json:"queued"`
		Pending            int `json:"pending"`
		ApproxContextBytes int `json:"approx_context_bytes"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if got.Sessions != 1 || got.Fired != 1 || got.Queued != 1 || got.Pending != 1 || got.ApproxContextBytes != 0 {
		t.Fatalf("JSON summary mismatch: %+v", got)
	}
}
