package main

import (
	"fmt"
	"strings"
)

// schemaDefinition is embedded in every prompt so the LLM knows the exact shape to return.
const schemaDefinition = `{
  "summary": "string — one paragraph describing what needs to be built",
  "requirements": ["string", "..."],
  "affected_components": ["string — file or package name", "..."],
  "proposed_changes": [
    {"file": "string", "description": "string"}
  ],
  "edge_cases": ["string", "..."],
  "open_questions": ["string", "..."]
}`

// BuildPrompt constructs the full LLM prompt from the issue and retrieved code snippets.
func BuildPrompt(issue GitHubIssue, snippets []CodeSnippet) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software engineer. Your job is to read a GitHub issue and relevant code context, then produce a strict engineering specification.\n\n")

	sb.WriteString("## GitHub Issue\n\n")
	fmt.Fprintf(&sb, "**Title:** %s\n\n", issue.Title)
	fmt.Fprintf(&sb, "**Body:**\n%s\n\n", issue.Body)

	if len(snippets) > 0 {
		sb.WriteString("## Relevant Code Context\n\n")
		for _, s := range snippets {
			fmt.Fprintf(&sb, "### File: `%s`\n```go\n%s\n```\n\n", s.FilePath, s.Content)
		}
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Based on the issue and code context above, produce a structured engineering specification.\n")
	sb.WriteString("You MUST respond with ONLY valid JSON — no explanation, no markdown fences, no extra text.\n\n")
	sb.WriteString("The JSON MUST conform exactly to this schema:\n\n")
	sb.WriteString(schemaDefinition)
	sb.WriteString("\n\nRemember: respond with ONLY the raw JSON object.")

	return sb.String()
}
