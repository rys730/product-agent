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
// repoTree and readme are optional; pass empty strings to omit them.
// codebaseIndex is optional markdown content describing the codebase; pass empty string to omit.
func BuildPrompt(issue GitHubIssue, snippets []CodeSnippet, repoTree, readme, codebaseIndex string) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software engineer. Your job is to read a GitHub issue and relevant code context, then produce a strict engineering specification.\n\n")

	if readme != "" {
		sb.WriteString("## Repository README\n\n")
		sb.WriteString(readme)
		sb.WriteString("\n\n")
	}

	if repoTree != "" {
		sb.WriteString("## Repository File Tree\n\n```\n")
		sb.WriteString(repoTree)
		sb.WriteString("```\n\n")
	}

	if codebaseIndex != "" {
		sb.WriteString("## Codebase Index\n\n")
		sb.WriteString(codebaseIndex)
		sb.WriteString("\n\n")
	}

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
