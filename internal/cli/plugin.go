package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const hookCommand = "tm check-action --hook"
const hookMatcher = "Edit|Write|MultiEdit"

// installClaudeCodeHook writes the PreToolUse hook entry to
// <repoDir>/.claude/settings.json. Returns (true, nil) when the entry was
// added, (false, nil) when .claude/ doesn't exist or the entry was already
// present.
func installClaudeCodeHook(repoDir string) (bool, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
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

	if hasHookEntry(settings) {
		return false, nil
	}

	addHookEntry(settings)

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func hasHookEntry(settings map[string]any) bool {
	return countHookEntries(settings) > 0
}

func countHookEntries(settings map[string]any) int {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return 0
	}
	preToolUse, _ := hooks["PreToolUse"].([]any)
	count := 0
	for _, entry := range preToolUse {
		group, _ := entry.(map[string]any)
		if group == nil {
			continue
		}
		if group["matcher"] == hookMatcher {
			inner, _ := group["hooks"].([]any)
			for _, h := range inner {
				hm, _ := h.(map[string]any)
				if hm["command"] == hookCommand {
					count++
				}
			}
		}
	}
	return count
}

func addHookEntry(settings map[string]any) {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	preToolUse, _ := hooks["PreToolUse"].([]any)
	entry := map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookCommand,
			},
		},
	}
	hooks["PreToolUse"] = append(preToolUse, entry)
}
