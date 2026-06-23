// Package safety checks proposed memories for content that should not be
// committed to the append-only ledger (prd.md §17).
package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

type Category string

const (
	CategorySecret Category = "secret"
	CategoryPII    Category = "PII"
)

type Finding struct {
	Category Category
	Field    string
	Rule     string
}

type Decision struct {
	Block []Finding
	Warn  []Finding
}

type fieldValue struct {
	name  string
	value string
}

type rule struct {
	category Category
	name     string
	re       *regexp.Regexp
}

var rules = []rule{
	{CategorySecret, "github_token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`)},
	{CategorySecret, "openai_key", regexp.MustCompile(`\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,}\b`)},
	{CategorySecret, "slack_token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{20,}\b`)},
	{CategorySecret, "aws_access_key_id", regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{CategorySecret, "private_key_header", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{CategorySecret, "secret_assignment", regexp.MustCompile(`(?i)\b(?:api[_-]?key|token|secret|password|passwd|credential)\b\s*[:=]\s*["']?[A-Za-z0-9_./+=-]{16,}`)},
	{CategoryPII, "email", regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)},
	{CategoryPII, "us_ssn", regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
}

func ScanMemory(m model.Memory) []Finding {
	var findings []Finding
	for _, fv := range memoryFields(m) {
		for _, r := range rules {
			if r.re.MatchString(fv.value) {
				findings = append(findings, Finding{
					Category: r.category,
					Field:    fv.name,
					Rule:     r.name,
				})
			}
		}
	}
	return findings
}

func FormatFindings(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "%s detected in %s (%s)\n", f.Category, f.Field, f.Rule)
	}
	return strings.TrimRight(b.String(), "\n")
}

func Decide(findings []Finding, secretAction, piiAction string) Decision {
	var d Decision
	for _, f := range findings {
		switch actionFor(f.Category, secretAction, piiAction) {
		case "block":
			d.Block = append(d.Block, f)
		case "warn":
			d.Warn = append(d.Warn, f)
		}
	}
	return d
}

func actionFor(category Category, secretAction, piiAction string) string {
	switch category {
	case CategorySecret:
		return normalizeAction(secretAction, "block")
	case CategoryPII:
		return normalizeAction(piiAction, "warn")
	default:
		return "off"
	}
}

func normalizeAction(action, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "block", "warn", "off":
		return strings.ToLower(strings.TrimSpace(action))
	default:
		return fallback
	}
}

func memoryFields(m model.Memory) []fieldValue {
	fields := []fieldValue{
		{name: "title", value: m.Title},
		{name: "summary", value: m.Summary},
		{name: "guidance", value: m.Guidance},
	}
	for i, ev := range m.Evidence {
		fields = append(fields,
			fieldValue{name: fmt.Sprintf("evidence[%d].type", i), value: ev.Type},
			fieldValue{name: fmt.Sprintf("evidence[%d].description", i), value: ev.Description},
			fieldValue{name: fmt.Sprintf("evidence[%d].ref", i), value: ev.Ref},
		)
	}
	return fields
}
