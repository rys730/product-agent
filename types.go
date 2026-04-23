package main

import "context"

// GitHubIssue holds the extracted issue data from a webhook payload.
type GitHubIssue struct {
	Owner  string
	Repo   string
	Number int
	Title  string
	Body   string
}

// Retriever is the interface both the local and GitHub API backends implement.
type Retriever interface {
	Retrieve(ctx context.Context, issue GitHubIssue, keywords []string) ([]CodeSnippet, error)
}

// CodeSnippet represents a retrieved file with its content.
type CodeSnippet struct {
	FilePath string
	Content  string
}

// ProposedChange describes a single code-level change recommendation.
type ProposedChange struct {
	File        string `json:"file"`
	Description string `json:"description"`
}

// AgentOutput is the structured specification produced by the LLM.
type AgentOutput struct {
	Summary            string           `json:"summary"`
	Requirements       []string         `json:"requirements"`
	AffectedComponents []string         `json:"affected_components"`
	ProposedChanges    []ProposedChange `json:"proposed_changes"`
	EdgeCases          []string         `json:"edge_cases"`
	OpenQuestions      []string         `json:"open_questions"`
}

// webhookPayload mirrors the GitHub webhook JSON for "issues" events.
type webhookPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	} `json:"issue"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	// Changes is populated by GitHub on "edited" actions.
	Changes struct {
		Title struct {
			From string `json:"from"`
		} `json:"title"`
		Body struct {
			From string `json:"from"`
		} `json:"body"`
	} `json:"changes"`
}
