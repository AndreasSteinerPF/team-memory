package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// hookSpec describes one Claude Code hook entry tm installs into
// .claude/settings.json.
type hookSpec struct {
	event   string // key under "hooks": PreToolUse | SessionStart
	matcher string // empty = no matcher field (SessionStart applies to all)
	command string
}

// claudeHookSpecs is everything `tm init` installs (prd.md §10.1): the
// edit-time check hook and the session-start briefing.
var claudeHookSpecs = []hookSpec{
	{event: "PreToolUse", matcher: "Edit|Write|MultiEdit|Bash", command: "tm check-action --hook"},
	{event: "SessionStart", matcher: "", command: "tm brief"},
	{event: "PostToolUse", matcher: "Edit|Write|MultiEdit|Bash", command: "tm signal --hook"},
	{event: "Stop", matcher: "", command: "tm nudge --hook"},
	{event: "UserPromptSubmit", matcher: "", command: "tm signal --hook --prompt"},
}

// installClaudeCodeHooks writes tm's hook entries to
// <repoDir>/.claude/settings.json. Returns (true, nil) when at least one entry
// was added, (false, nil) when .claude/ doesn't exist or all entries were
// already present.
func installClaudeCodeHooks(repoDir string) (bool, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	if _, err := os.Stat(claudeDir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, err
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}

	added := false
	for _, spec := range claudeHookSpecs {
		if countHookEntries(settings, spec) == 0 {
			addHookEntry(settings, spec)
			added = true
		}
	}
	if !added {
		return false, nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func countHookEntries(settings map[string]any, spec hookSpec) int {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return 0
	}
	entries, _ := hooks[spec.event].([]any)
	count := 0
	for _, entry := range entries {
		group, _ := entry.(map[string]any)
		if group == nil {
			continue
		}
		matcher, _ := group["matcher"].(string)
		if matcher != spec.matcher {
			continue
		}
		inner, _ := group["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if hm["command"] == spec.command {
				count++
			}
		}
	}
	return count
}

func addHookEntry(settings map[string]any, spec hookSpec) {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	existing, _ := hooks[spec.event].([]any)
	group := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": spec.command,
			},
		},
	}
	if spec.matcher != "" {
		group["matcher"] = spec.matcher
	}
	hooks[spec.event] = append(existing, group)
}
