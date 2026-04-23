package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// GitHubRetriever fetches repository files via the GitHub REST API.
// It uses the Trees API to list paths, then the Contents API to fetch content.
type GitHubRetriever struct {
	token      string
	extensions []string
	branch     string // empty = use repo's default branch
	http       *http.Client
}

// NewGitHubRetriever creates a GitHubRetriever.
// If branch is empty the repo's default branch is used automatically.
func NewGitHubRetriever(token string, extensions []string, branch string) *GitHubRetriever {
	return &GitHubRetriever{
		token:      token,
		extensions: extensions,
		branch:     branch,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Retrieve implements the Retriever interface using the GitHub API.
// Phase 1: score all paths by keyword matches in the path → take top 20.
// Phase 2: fetch content for those 20, re-score by content frequency → top 5.
func (g *GitHubRetriever) Retrieve(ctx context.Context, issue GitHubIssue, keywords []string) ([]CodeSnippet, error) {
	// Build allowed extension set.
	extSet := make(map[string]bool, len(g.extensions))
	for _, e := range g.extensions {
		extSet[strings.ToLower(e)] = true
	}

	// 1. Fetch the full recursive tree.
	paths, err := g.listTree(ctx, issue.Owner, issue.Repo)
	if err != nil {
		return nil, fmt.Errorf("list tree: %w", err)
	}
	log.Printf("[retriever/gh] %d files in tree for %s/%s", len(paths), issue.Owner, issue.Repo)

	// 2. Filter by extension and score by keyword matches in the path.
	type scored struct {
		path    string
		score   int
		content string
	}
	var candidates []scored
	for _, p := range paths {
		ext := fileExt(p)
		if !extSet[ext] {
			continue
		}
		score := 0
		lower := strings.ToLower(p)
		for _, kw := range keywords {
			score += strings.Count(lower, kw)
		}
		candidates = append(candidates, scored{path: p, score: score})
	}

	// Sort descending by path score.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	// Pre-filter: take top 20 by path score for content fetching.
	const preFilterN = 20
	if len(candidates) > preFilterN {
		candidates = candidates[:preFilterN]
	}

	// 3. Fetch content for each candidate and re-score by content frequency.
	for i := range candidates {
		content, err := g.fetchContent(ctx, issue.Owner, issue.Repo, candidates[i].path)
		if err != nil {
			log.Printf("[retriever/gh] skipping %s: %v", candidates[i].path, err)
			continue
		}
		candidates[i].content = content
		lower := strings.ToLower(content)
		score := 0
		for _, kw := range keywords {
			score += strings.Count(lower, kw)
		}
		candidates[i].score = score
		log.Printf("[retriever/gh] content-scored %s (score=%d)", candidates[i].path, score)
	}

	// Re-sort by content score.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	// Take top N.
	if len(candidates) > maxResults {
		candidates = candidates[:maxResults]
	}

	snippets := make([]CodeSnippet, 0, len(candidates))
	for _, c := range candidates {
		if c.content == "" {
			continue
		}
		snippets = append(snippets, CodeSnippet{
			FilePath: c.path,
			Content:  truncateLines(c.content, maxFileLines),
		})
		log.Printf("[retriever/gh] selected %s (score=%d)", c.path, c.score)
	}

	return snippets, nil
}

// listTree calls the Git Trees API (recursive) and returns all blob paths.
func (g *GitHubRetriever) listTree(ctx context.Context, owner, repo string) ([]string, error) {
	branch := g.branch

	// If no branch configured, resolve the default branch via the repo endpoint.
	if branch == "" {
		repoURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
		repoResp, err := g.apiGet(ctx, repoURL)
		if err != nil {
			return nil, err
		}
		var repoInfo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.Unmarshal(repoResp, &repoInfo); err != nil {
			return nil, fmt.Errorf("parse repo info: %w", err)
		}
		branch = repoInfo.DefaultBranch
		if branch == "" {
			branch = "main"
		}
	}

	log.Printf("[retriever/gh] using branch: %s", branch)

	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, branch)
	treeResp, err := g.apiGet(ctx, treeURL)
	if err != nil {
		return nil, err
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(treeResp, &tree); err != nil {
		return nil, fmt.Errorf("parse tree: %w", err)
	}
	if tree.Truncated {
		log.Printf("[retriever/gh] warning: tree was truncated by GitHub (repo too large)")
	}

	paths := make([]string, 0, len(tree.Tree))
	for _, node := range tree.Tree {
		if node.Type == "blob" {
			paths = append(paths, node.Path)
		}
	}
	return paths, nil
}

// fetchContent retrieves and decodes a single file via the Contents API.
func (g *GitHubRetriever) fetchContent(ctx context.Context, owner, repo, path string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)
	data, err := g.apiGet(ctx, url)
	if err != nil {
		return "", err
	}

	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return "", fmt.Errorf("parse content: %w", err)
	}
	if file.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding: %s", file.Encoding)
	}

	// GitHub's base64 includes newlines — strip them before decoding.
	raw := strings.ReplaceAll(file.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	return string(decoded), nil
}

// apiGet performs an authenticated GET to the GitHub API.
func (g *GitHubRetriever) apiGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api %s → %d: %s", url, resp.StatusCode, string(body))
	}
	return body, nil
}

// fileExt returns the lowercase extension of a path, e.g. "src/main.go" → ".go".
func fileExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return strings.ToLower(path[i:])
		}
		if path[i] == '/' {
			break
		}
	}
	return ""
}

// FetchREADME fetches the repository README via the GitHub Contents API.
// Returns empty string if not found or on error.
func (g *GitHubRetriever) FetchREADME(ctx context.Context, owner, repo string) string {
	content, err := g.fetchContent(ctx, owner, repo, "README.md")
	if err != nil {
		return ""
	}
	return truncateLines(content, 100)
}

// FetchProductAgentMD fetches PRODUCT-AGENT.md from the repo root via the GitHub Contents API.
// Returns empty string if not found or on error.
func (g *GitHubRetriever) FetchProductAgentMD(ctx context.Context, owner, repo string) string {
	content, err := g.fetchContent(ctx, owner, repo, "PRODUCT-AGENT.md")
	if err != nil {
		return ""
	}
	return content
}
