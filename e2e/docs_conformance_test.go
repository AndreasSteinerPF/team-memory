package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	prdCitationGroupRE = regexp.MustCompile("prd\\.md §§?([^`\\n)]+)")
	prdSectionNumberRE = regexp.MustCompile(`\d+(?:\.\d+)?`)
)

func TestTechnicalDocsConformance(t *testing.T) {
	repo := filepath.Join("..")
	readme := mustReadDoc(t, filepath.Join(repo, "README.md"))
	prd := mustReadDoc(t, filepath.Join(repo, "prd.md"))

	docs := []string{
		"docs/architecture.md",
		"docs/design-principles.md",
		"docs/threat-model.md",
		"docs/evaluation.md",
		"docs/roadmap.md",
	}
	assertTechnicalDocLinksResolve(t, repo, readme, docs)
	assertREADMEHarnessValues(t, repo, readme)
	assertREADMEMCPTools(t, repo, readme)
	assertTechnicalDocsArePRDProjections(t, repo, prd, docs)
}

func mustReadDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertTechnicalDocLinksResolve(t *testing.T, repo, readme string, want []string) {
	t.Helper()
	sectionRE := regexp.MustCompile(`(?ms)^## Technical docs\s*\n(.*?)(?:^## |\z)`)
	section := sectionRE.FindStringSubmatch(readme)
	if section == nil {
		t.Fatal("README is missing a Technical docs section")
	}
	linkRE := regexp.MustCompile(`\[[^\]]+\]\((docs/[^)#]+\.md)\)`)
	var got []string
	for _, match := range linkRE.FindAllStringSubmatch(section[1], -1) {
		got = append(got, filepath.ToSlash(match[1]))
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(match[1]))); err != nil {
			t.Errorf("README technical-doc link %q does not resolve: %v", match[1], err)
		}
	}
	sort.Strings(got)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(got, sortedWant) {
		t.Errorf("README technical-doc links = %v, want %v", got, sortedWant)
	}
}

func assertREADMEHarnessValues(t *testing.T, repo, readme string) {
	t.Helper()
	re := regexp.MustCompile("`tm init --harness \\{([^}]+)\\}`")
	match := re.FindStringSubmatch(readme)
	if match == nil {
		t.Fatal("README is missing the tm init --harness value list")
	}
	got := strings.Split(match[1], ",")
	initSource := mustReadDoc(t, filepath.Join(repo, "internal", "cli", "init.go"))
	caseRE := regexp.MustCompile(`case "", "claude", ([^:]+):`)
	caseMatch := caseRE.FindStringSubmatch(initSource)
	if caseMatch == nil {
		t.Fatal("cannot find supported harness case in internal/cli/init.go")
	}
	nameRE := regexp.MustCompile(`"([^"]+)"`)
	var want []string
	for _, name := range nameRE.FindAllStringSubmatch(caseMatch[1], -1) {
		want = append(want, name[1])
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("README harness values = %v, want %v", got, want)
	}
}

func assertREADMEMCPTools(t *testing.T, repo, readme string) {
	t.Helper()
	re := regexp.MustCompile(`MCP tools:\s*([^\n]+)`)
	match := re.FindStringSubmatch(readme)
	if match == nil {
		t.Fatal("README is missing the MCP tools list")
	}
	nameRE := regexp.MustCompile("`(tm_[a-z_]+)`")
	var got []string
	for _, name := range nameRE.FindAllStringSubmatch(match[1], -1) {
		got = append(got, name[1])
	}
	serverSource := mustReadDoc(t, filepath.Join(repo, "internal", "mcp", "server.go"))
	toolRE := regexp.MustCompile(`Name:\s+"(tm_[a-z_]+)"`)
	var want []string
	for _, name := range toolRE.FindAllStringSubmatch(serverSource, -1) {
		want = append(want, name[1])
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("README MCP tools = %v, want %v", got, want)
	}
}

func assertTechnicalDocsArePRDProjections(t *testing.T, repo, prd string, docs []string) {
	t.Helper()
	sections := prdSections(prd)
	for _, path := range docs {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			body := mustReadDoc(t, filepath.Join(repo, filepath.FromSlash(path)))
			if !strings.Contains(strings.ToLower(body), "explanatory projection") {
				t.Error("document must identify itself as an explanatory projection of prd.md")
			}
			citations := citedPRDSections(body)
			if len(citations) == 0 {
				t.Error("document must contain at least one prd.md §X.Y citation")
			}
			for _, citation := range citations {
				if _, ok := sections[citation]; !ok {
					t.Errorf("citation prd.md §%s does not name an existing PRD section", citation)
				}
			}
		})
	}
}

func citedPRDSections(body string) []string {
	var out []string
	for _, group := range prdCitationGroupRE.FindAllStringSubmatch(body, -1) {
		out = append(out, prdSectionNumberRE.FindAllString(group[1], -1)...)
	}
	return out
}

func prdSections(prd string) map[string]struct{} {
	re := regexp.MustCompile(`(?m)^#{2,3}\s+(\d+(?:\.\d+)?)\b`)
	sections := make(map[string]struct{})
	for _, match := range re.FindAllStringSubmatch(prd, -1) {
		sections[match[1]] = struct{}{}
	}
	return sections
}
