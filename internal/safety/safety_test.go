package safety_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/safety"
)

func TestScanMemoryFindsSecretsAndPII(t *testing.T) {
	m := model.Memory{
		Title:    "Do not store sk-proj-1234567890abcdef1234567890abcdef",
		Summary:  "Contact alice@example.com after rotation",
		Guidance: "Use the replacement credential from the vault",
		Evidence: []model.Evidence{{Type: "log", Ref: "logs/run.txt"}},
	}

	findings := safety.ScanMemory(m)

	if !hasFinding(findings, safety.CategorySecret, "title") {
		t.Fatalf("missing secret finding in title: %#v", findings)
	}
	if !hasFinding(findings, safety.CategoryPII, "summary") {
		t.Fatalf("missing PII finding in summary: %#v", findings)
	}
}

func TestScanMemoryFindsEvidenceRefSecrets(t *testing.T) {
	m := model.Memory{
		Title: "safe title",
		Evidence: []model.Evidence{{
			Type: "log",
			Ref:  "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
		}},
	}

	findings := safety.ScanMemory(m)

	if !hasFinding(findings, safety.CategorySecret, "evidence[0].ref") {
		t.Fatalf("missing secret finding in evidence ref: %#v", findings)
	}
}

func TestFormatFindingsDoesNotEchoSecret(t *testing.T) {
	const secret = "ghp_1234567890abcdef1234567890abcdef1234"
	m := model.Memory{Title: "token = " + secret}

	msg := safety.FormatFindings(safety.ScanMemory(m))

	if strings.Contains(msg, secret) {
		t.Fatalf("message echoed secret: %q", msg)
	}
	if !strings.Contains(msg, "title") || !strings.Contains(strings.ToLower(msg), "secret") {
		t.Fatalf("message lacks useful context: %q", msg)
	}
}

func TestDecideFailsClosedForInvalidSecretAction(t *testing.T) {
	findings := []safety.Finding{{Category: safety.CategorySecret, Field: "title", Rule: "openai_key"}}

	decision := safety.Decide(findings, "blokc", "warn")

	if len(decision.Block) != 1 {
		t.Fatalf("invalid secret action should block, got block=%v warn=%v", decision.Block, decision.Warn)
	}
}

func hasFinding(findings []safety.Finding, category safety.Category, field string) bool {
	for _, f := range findings {
		if f.Category == category && f.Field == field {
			return true
		}
	}
	return false
}
