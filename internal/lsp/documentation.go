package lsp

import "strings"

// summaryBefore extracts a Delphi XML documentation summary immediately preceding
// a declaration. It accepts both /// comments and brace-style Delphi comments.
func summaryBefore(lines []string, declarationLine int) string {
	var collected []string
	inside := false
	for i := declarationLine - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		lower := strings.ToLower(line)
		if !inside {
			end := strings.Index(lower, "</summary>")
			if end < 0 {
				if line == "" {
					continue
				}
				return ""
			}
			inside = true
			line = line[:end]
			lower = strings.ToLower(line)
		}
		start := strings.Index(lower, "<summary>")
		if start >= 0 {
			collected = append(collected, cleanDocLine(line[start+len("<summary>"):]))
			break
		}
		collected = append(collected, cleanDocLine(line))
	}
	if !inside {
		return ""
	}
	for left, right := 0, len(collected)-1; left < right; left, right = left+1, right-1 {
		collected[left], collected[right] = collected[right], collected[left]
	}
	return strings.TrimSpace(strings.Join(collected, "\n"))
}

func cleanDocLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "///")
	line = strings.TrimPrefix(line, "{")
	line = strings.TrimPrefix(line, "(*")
	line = strings.TrimSuffix(line, "}")
	line = strings.TrimSuffix(line, "*)")
	return strings.TrimSpace(line)
}
