package orchestrator

import (
	"encoding/json"
	"strings"
)

// parseJSONObjects extracts one or more JSON objects from text
// Handles the case where the judge returns multiple JSON objects in one response
func parseJSONObjects(text string) []map[string]interface{} {
	var objects []map[string]interface{}
	if text == "" {
		return objects
	}

	// First, try parsing the whole text as a single JSON object (fast path)
	stripped := strings.TrimSpace(text)
	if strings.HasPrefix(stripped, "{") {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(stripped), &obj); err == nil {
			return []map[string]interface{}{obj}
		}
	}

	// Find all top-level JSON objects by tracking brace depth
	i := 0
	for i < len(text) {
		if text[i] == '{' {
			depth := 0
			inString := false
			escapeNext := false
			start := i

			for j := i; j < len(text); j++ {
				ch := text[j]

				if escapeNext {
					escapeNext = false
					continue
				}

				if ch == '\\' && inString {
					escapeNext = true
					continue
				}

				if ch == '"' && !escapeNext {
					inString = !inString
					continue
				}

				if inString {
					continue
				}

				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
					if depth == 0 {
						candidate := text[start : j+1]
						var obj map[string]interface{}
						if err := json.Unmarshal([]byte(candidate), &obj); err == nil {
							objects = append(objects, obj)
						}
						i = j + 1
						break
					}
				}
			}

			// Unbalanced braces, skip this opening brace
			if depth != 0 {
				i++
			}
		} else {
			i++
		}
	}

	return objects
}
