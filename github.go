package main

import (
"bytes"
"context"
"encoding/json"
"fmt"
"io"
"log"
"net/http"
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
