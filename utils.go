package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
)

// parseExtensions splits a comma-separated extension string into a cleaned
// slice, ensuring each entry starts with a dot and is lowercase.
// e.g. ".go, .ts, py" → [".go", ".ts", ".py"]
func parseExtensions(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		ext := strings.ToLower(strings.TrimSpace(p))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		out = append(out, ext)
	}
	return out
}

// parseActions splits a comma-separated actions string into a set (map).
// e.g. "opened,edited" → {"opened": true, "edited": true}
func parseActions(raw string) map[string]bool {
	parts := strings.Split(raw, ",")
	out := make(map[string]bool, len(parts))
	for _, p := range parts {
		a := strings.ToLower(strings.TrimSpace(p))
		if a != "" {
			out[a] = true
		}
	}
	return out
}

// loadDotEnv reads a .env file and sets any variables not already present in
// the environment. It is a no-op if the file does not exist.
// Rules:
//   - Lines starting with # are comments.
//   - Blank lines are ignored.
//   - Format: KEY=VALUE  (leading/trailing whitespace stripped from both).
//   - Inline comments (# ...) are NOT stripped — values are taken as-is after the '='.
//   - Existing env vars are never overwritten.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return // silently skip if no .env file
		}
		log.Printf("[env] could not open %s: %v", path, err)
		return
	}
	defer f.Close()

	loaded := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue // no '=' or key is empty
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip optional surrounding quotes (" or ').
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Never override a variable already set in the real environment.
		if os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			log.Printf("[env] could not set %s: %v", key, err)
			continue
		}
		loaded++
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[env] error reading %s: %v", path, err)
	}
	if loaded > 0 {
		log.Printf("[env] loaded %d variable(s) from %s", loaded, path)
	}
}

// stopwords is a minimal set of common English words to filter out.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "it": true, "as": true,
	"be": true, "was": true, "are": true, "were": true, "has": true, "have": true,
	"had": true, "not": true, "this": true, "that": true, "we": true, "i": true,
	"when": true, "if": true, "so": true, "do": true, "can": true, "should": true,
	"would": true, "will": true, "my": true, "our": true, "they": true, "their": true,
	"he": true, "she": true, "you": true, "your": true, "its": true, "which": true,
	"also": true, "than": true, "then": true, "into": true, "about": true, "up": true,
	"out": true, "no": true, "what": true, "how": true, "all": true, "any": true,
}

// ExtractKeywords lowercases the text, strips punctuation, removes stopwords,
// deduplicates, and returns meaningful words of at least 3 characters.
func ExtractKeywords(text string) []string {
	// Normalize: lowercase and replace non-letter/digit runs with spaces.
	var sb strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}

	parts := strings.Fields(sb.String())
	seen := make(map[string]bool)
	keywords := make([]string, 0, len(parts))

	for _, w := range parts {
		if len(w) < 3 {
			continue
		}
		if stopwords[w] {
			continue
		}
		if seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}

	return keywords
}

// FormatComment converts an AgentOutput into a readable GitHub Markdown comment.
func FormatComment(out AgentOutput) string {
	var sb strings.Builder

	sb.WriteString("## 🤖 Product Agent — Engineering Spec\n\n")

	sb.WriteString("### 📋 Summary\n")
	sb.WriteString(out.Summary)
	sb.WriteString("\n\n")

	sb.WriteString("### ✅ Requirements\n")
	for _, r := range out.Requirements {
		fmt.Fprintf(&sb, "- %s\n", r)
	}
	sb.WriteString("\n")

	sb.WriteString("### 🗂 Affected Components\n")
	for _, c := range out.AffectedComponents {
		fmt.Fprintf(&sb, "- `%s`\n", c)
	}
	sb.WriteString("\n")

	sb.WriteString("### 🔧 Proposed Changes\n")
	for _, p := range out.ProposedChanges {
		fmt.Fprintf(&sb, "- **`%s`**: %s\n", p.File, p.Description)
	}
	sb.WriteString("\n")

	sb.WriteString("### ⚠️ Edge Cases\n")
	for _, e := range out.EdgeCases {
		fmt.Fprintf(&sb, "- %s\n", e)
	}
	sb.WriteString("\n")

	sb.WriteString("### ❓ Open Questions\n")
	for _, q := range out.OpenQuestions {
		fmt.Fprintf(&sb, "- %s\n", q)
	}
	sb.WriteString("\n")

	sb.WriteString("---\n*Generated by [Product Agent](https://github.com/product-agent)*\n")

	return sb.String()
}

// truncateLines returns at most maxLines lines of content joined back together.
func truncateLines(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	truncated := lines[:maxLines]
	truncated = append(truncated, fmt.Sprintf("... (%d lines truncated)", len(lines)-maxLines))
	return strings.Join(truncated, "\n")
}

// minNewKeywords is the minimum number of new keywords required to consider
// an issue edit significant enough to reprocess.
const minNewKeywords = 3

// isSignificantEdit returns true if the change between oldText and newText
// introduces enough new keywords to warrant reprocessing.
// It compares the keyword sets of both texts and counts keywords that are
// present in the new text but absent in the old.
func isSignificantEdit(oldText, newText string) bool {
	oldKws := make(map[string]bool)
	for _, k := range ExtractKeywords(oldText) {
		oldKws[k] = true
	}

	newCount := 0
	for _, k := range ExtractKeywords(newText) {
		if !oldKws[k] {
			newCount++
		}
	}
	return newCount >= minNewKeywords
}
