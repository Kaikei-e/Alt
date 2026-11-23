package guards

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// HookPayload is a watched PreToolUse edit payload.
type HookPayload struct {
	ToolName  string
	Paths     []string
	AddedText map[string]string
	Cwd       string
}

var watchedEditTools = map[string]struct{}{
	"Edit": {}, "Write": {}, "NotebookEdit": {}, "apply_patch": {},
}

var patchOpenPrefixes = []string{
	"*** Add File: ",
	"*** Update File: ",
	"*** Delete File: ",
}

const patchMovePrefix = "*** Move to: "

// ParseHookStdin parses a PreToolUse JSON body.
// ok is false when the tool is not watched (caller exits 0).
func ParseHookStdin(raw string) (HookPayload, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return HookPayload{}, false, fmt.Errorf("guard could not read the hook payload: invalid JSON (%s)", jsonErrMsg(err))
	}
	if payload == nil {
		return HookPayload{}, false, fmt.Errorf("guard could not read the hook payload: expected a JSON object")
	}
	tool, _ := payload["tool_name"].(string)
	if strings.TrimSpace(tool) == "" {
		return HookPayload{}, false, fmt.Errorf("guard could not read the hook payload: missing tool_name")
	}
	if _, watch := watchedEditTools[tool]; !watch {
		return HookPayload{}, false, nil
	}
	input, ok := payload["tool_input"].(map[string]any)
	if !ok {
		return HookPayload{}, false, fmt.Errorf("guard could not read the hook payload: missing tool_input for %s", tool)
	}
	var ordered []string
	seen := map[string]struct{}{}
	added := map[string]string{}
	claudeAdded := addedTextForClaude(tool, input)
	for _, field := range []string{"file_path", "notebook_path"} {
		if value, ok := input[field].(string); ok && strings.TrimSpace(value) != "" {
			cleaned := strings.TrimSpace(value)
			appendUnique(&ordered, seen, cleaned)
			added[cleaned] = claudeAdded
		}
	}
	if tool == "apply_patch" {
		collectApplyPatch(input, &ordered, seen, added)
	}
	if len(ordered) == 0 {
		return HookPayload{}, false, fmt.Errorf("guard could not read the hook payload: %s yielded no target path", tool)
	}
	cwd := ""
	if rawCwd, ok := payload["cwd"].(string); ok && strings.TrimSpace(rawCwd) != "" {
		cwd = strings.TrimSpace(rawCwd)
	}
	return HookPayload{ToolName: tool, Paths: ordered, AddedText: added, Cwd: cwd}, true, nil
}

func appendUnique(ordered *[]string, seen map[string]struct{}, path string) {
	if _, ok := seen[path]; ok {
		return
	}
	seen[path] = struct{}{}
	*ordered = append(*ordered, path)
}

func addedTextForClaude(tool string, input map[string]any) string {
	switch tool {
	case "Write":
		if v, ok := input["content"].(string); ok {
			return v
		}
	case "Edit":
		if v, ok := input["new_string"].(string); ok {
			return v
		}
	case "NotebookEdit":
		for _, key := range []string{"new_source", "content"} {
			if v, ok := input[key].(string); ok {
				return v
			}
		}
	}
	return ""
}

func collectApplyPatch(input map[string]any, ordered *[]string, seen map[string]struct{}, added map[string]string) {
	var patchText string
	switch cmd := input["command"].(type) {
	case string:
		patchText = cmd
	case []any:
		var parts []string
		for _, part := range cmd {
			if s, ok := part.(string); ok {
				parts = append(parts, s)
			}
		}
		patchText = strings.Join(parts, "\n")
	}
	paths, patchAdded := extractPatchDetails(patchText)
	for _, path := range paths {
		appendUnique(ordered, seen, path)
		if prev, ok := added[path]; ok {
			bits := []string{}
			if prev != "" {
				bits = append(bits, prev)
			}
			if patchAdded[path] != "" {
				bits = append(bits, patchAdded[path])
			}
			added[path] = strings.Join(bits, "\n")
		} else {
			added[path] = patchAdded[path]
		}
	}
}

func extractPatchDetails(commandText string) ([]string, map[string]string) {
	var ordered []string
	seen := map[string]struct{}{}
	linesBy := map[string][]string{}
	var current []string
	for _, line := range strings.Split(commandText, "\n") {
		matched := false
		for _, prefix := range patchOpenPrefixes {
			if strings.HasPrefix(line, prefix) {
				candidate := strings.TrimSpace(line[len(prefix):])
				if candidate != "" {
					appendUnique(&ordered, seen, candidate)
					if _, ok := linesBy[candidate]; !ok {
						linesBy[candidate] = nil
					}
					current = []string{candidate}
				} else {
					current = nil
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if strings.HasPrefix(line, patchMovePrefix) {
			candidate := strings.TrimSpace(line[len(patchMovePrefix):])
			if candidate != "" {
				appendUnique(&ordered, seen, candidate)
				if _, ok := linesBy[candidate]; !ok {
					linesBy[candidate] = nil
				}
				found := false
				for _, c := range current {
					if c == candidate {
						found = true
						break
					}
				}
				if !found {
					current = append(current, candidate)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "*** ") {
			current = nil
			continue
		}
		if strings.HasPrefix(line, "+") && len(current) > 0 {
			added := line[1:]
			for _, target := range current {
				linesBy[target] = append(linesBy[target], added)
			}
		}
	}
	out := map[string]string{}
	for p, lines := range linesBy {
		out[p] = strings.Join(lines, "\n")
	}
	return ordered, out
}

func readHook(stdin io.Reader, stderr io.Writer) (*HookPayload, int) {
	if stdin == nil {
		fmt.Fprintln(stderr, "guard could not read the hook payload: invalid JSON (empty)")
		return nil, 2
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "guard could not read the hook payload: invalid JSON (%v)\n", err)
		return nil, 2
	}
	payload, ok, err := ParseHookStdin(string(raw))
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return nil, 2
	}
	if !ok {
		return nil, 0
	}
	return &payload, 0
}
