package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const githubAPIBase = "https://api.github.com"

// GitHubClient handles communication with the GitHub REST API.
type GitHubClient struct {
	token string
	http  *http.Client
}

// NewGitHubClient creates a GitHubClient with the provided personal access token.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token: token,
		http:  &http.Client{},
	}
}

// ParseWebhookPayload decodes a raw GitHub "issues" webhook body into a GitHubIssue.
func ParseWebhookPayload(body []byte) (GitHubIssue, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return GitHubIssue{}, fmt.Errorf("unmarshal webhook: %w", err)
	}

	if p.Action == "" {
		return GitHubIssue{}, fmt.Errorf("missing action field")
	}
	if p.Repository.Name == "" || p.Repository.Owner.Login == "" {
		return GitHubIssue{}, fmt.Errorf("missing repository info")
	}
	if p.Issue.Number == 0 {
		return GitHubIssue{}, fmt.Errorf("missing issue number")
	}

	return GitHubIssue{
		Owner:  p.Repository.Owner.Login,
		Repo:   p.Repository.Name,
		Number: p.Issue.Number,
		Title:  p.Issue.Title,
		Body:   p.Issue.Body,
	}, nil
}

// parseOldText extracts the previous title+body from the "changes" field of an
// edited webhook payload. Returns empty string if not an edit or no changes present.
func parseOldText(body []byte) string {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return ""
	}
	return p.Changes.Title.From + " " + p.Changes.Body.From
}

// PostComment posts a Markdown comment on the given issue.
func (g *GitHubClient) PostComment(ctx context.Context, issue GitHubIssue, comment string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		githubAPIBase, issue.Owner, issue.Repo, issue.Number)

	body, err := json.Marshal(map[string]string{"body": comment})
	if err != nil {
		return fmt.Errorf("marshal comment: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("post comment http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api error %d: %s", resp.StatusCode, string(b))
	}

	log.Printf("[github] comment posted on %s/%s#%d", issue.Owner, issue.Repo, issue.Number)
	return nil
}

// agentCommentSignature is a string present in every comment we post.
// Used to detect whether we've already processed an issue.
const agentCommentSignature = "🤖 Product Agent"

// HasAgentComment returns true if the issue already has a comment posted by
// this agent (identified by agentCommentSignature).
func (g *GitHubClient) HasAgentComment(ctx context.Context, issue GitHubIssue) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100",
		githubAPIBase, issue.Owner, issue.Repo, issue.Number)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("list comments http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("github api error %d: %s", resp.StatusCode, string(b))
	}

	var comments []struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return false, fmt.Errorf("decode comments: %w", err)
	}

	for _, c := range comments {
		if strings.Contains(c.Body, agentCommentSignature) {
			return true, nil
		}
	}
	return false, nil
}
