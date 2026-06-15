package harness_e2e

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// ParseCapabilityMatrix extracts the ```capability-matrix fenced block from
// prd.md and returns harness name → declared CapabilitySet. The format is a
// pipe table: header row names capabilities, each data row is
// "<harness> | yes | no | …". Cells are "yes"/"no".
func ParseCapabilityMatrix(prd []byte) (map[string]CapabilitySet, error) {
	sc := bufio.NewScanner(bytes.NewReader(prd))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	inBlock := false
	var header []Capability
	out := map[string]CapabilitySet{}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		if !inBlock {
			if strings.TrimSpace(line) == "```capability-matrix" {
				inBlock = true
			}
			continue
		}
		if strings.TrimSpace(line) == "```" {
			break // end of block
		}
		cells := splitPipe(line)
		if len(cells) < 2 {
			continue
		}
		if header == nil {
			// header row: first cell is the "harness" label, rest are capabilities.
			for _, name := range cells[1:] {
				c, ok := ParseCapability(name)
				if !ok {
					return nil, fmt.Errorf("unknown capability column %q", name)
				}
				header = append(header, c)
			}
			continue
		}
		harness := cells[0]
		if len(cells)-1 != len(header) {
			return nil, fmt.Errorf("row %q has %d cells, want %d", harness, len(cells)-1, len(header))
		}
		set := CapabilitySet{}
		for i, c := range header {
			switch cells[i+1] {
			case "yes":
				set[c] = true
			case "no":
				// absent
			default:
				return nil, fmt.Errorf("harness %q capability %q: cell %q is not yes/no", harness, c, cells[i+1])
			}
		}
		out[harness] = set
	}
	if !inBlock {
		return nil, fmt.Errorf("no ```capability-matrix block found in prd.md")
	}
	if header == nil {
		return nil, fmt.Errorf("capability-matrix block had no header row")
	}
	return out, nil
}

// splitPipe splits a "a | b | c" row into trimmed cells.
func splitPipe(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
