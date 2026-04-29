package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// fixerFileOutput represents a file entry in PHP-CS-Fixer JSON output
type fixerFileOutput struct {
	Name string `json:"name"`
	Diff string `json:"diff"`
}

// fixerJsonOutput represents the full PHP-CS-Fixer JSON output
type fixerJsonOutput struct {
	Files []fixerFileOutput `json:"files"`
}

// diffHunk represents a single hunk extracted from a diff
type diffHunk struct {
	Line    int
	Context string
}

// extractHunksFromDiff parses a unified diff and returns hunks with line numbers and diff context
func extractHunksFromDiff(diff string) []diffHunk {
	hunks := []diffHunk{}
	lines := strings.Split(diff, "\n")

	for i, line := range lines {
		if !strings.HasPrefix(line, "@@") {
			continue
		}

		// Parse "@@ -10,5 +10,6 @@" format
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		lineInfo := strings.TrimPrefix(parts[1], "-")
		lineNum := 0
		fmt.Sscanf(lineInfo, "%d", &lineNum)
		if lineNum <= 0 {
			continue
		}

		// Collect changed lines for this hunk (until next @@ or end)
		var hunkLines []string
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "@@") {
				break
			}
			if strings.HasPrefix(lines[j], "-") || strings.HasPrefix(lines[j], "+") {
				hunkLines = append(hunkLines, lines[j])
			}
		}

		context := strings.Join(hunkLines, "\n")
		if context == "" {
			context = "style violation"
		}

		hunks = append(hunks, diffHunk{
			Line:    lineNum,
			Context: context,
		})
	}
	return hunks
}

// parseFixerJsonOutput parses PHP-CS-Fixer JSON stdout and returns detailed LintResults.
// Separates stdout (JSON) from stderr (progress/warnings) for reliable parsing.
func parseFixerJsonOutput(stdout, stderr []byte, ruleSlug, ruleName, fallbackPath string) []LintResult {
	results := []LintResult{}

	var fixerOutput fixerJsonOutput

	if jsonErr := json.Unmarshal(stdout, &fixerOutput); jsonErr == nil {
		for _, file := range fixerOutput.Files {
			cleanFile := strings.TrimPrefix(file.Name, "/app/")
			hunks := extractHunksFromDiff(file.Diff)

			if len(hunks) == 0 {
				// File has violations but no parseable hunks
				results = append(results, LintResult{
					Rule:     ruleSlug,
					File:     cleanFile,
					Line:     0,
					Column:   0,
					Message:  fmt.Sprintf("%s violation detected", ruleName),
					Severity: "error",
				})
				continue
			}

			for _, hunk := range hunks {
				results = append(results, LintResult{
					Rule:     ruleSlug,
					File:     cleanFile,
					Line:     hunk.Line,
					Column:   0,
					Message:  fmt.Sprintf("%s: %s", ruleName, hunk.Context),
					Severity: "error",
				})
			}
		}
	} else {
		// JSON parsing failed — use raw output as fallback
		rawOutput := string(stdout)
		if rawOutput == "" {
			rawOutput = string(stderr)
		}
		if len(rawOutput) > 500 {
			rawOutput = rawOutput[:500] + "..."
		}
		results = append(results, LintResult{
			Rule:     ruleSlug,
			File:     fallbackPath,
			Line:     0,
			Message:  fmt.Sprintf("%s: %s", ruleName, strings.TrimSpace(rawOutput)),
			Severity: "error",
		})
	}

	return results
}

