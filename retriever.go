package main

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxResults   = 5
	maxFileLines = 300
)

// LocalRetriever scans a local directory for relevant files.
type LocalRetriever struct {
	repoPath   string
	extensions []string
}

// NewLocalRetriever creates a LocalRetriever for the given path and extensions.
func NewLocalRetriever(repoPath string, extensions []string) *LocalRetriever {
	return &LocalRetriever{repoPath: repoPath, extensions: extensions}
}

// Retrieve implements the Retriever interface using local disk access.
func (r *LocalRetriever) Retrieve(_ context.Context, _ GitHubIssue, keywords []string) ([]CodeSnippet, error) {
	return RetrieveRelevantFiles(r.repoPath, keywords, r.extensions)
}

// RetrieveRelevantFiles scans repoPath for files whose extension is in the
// allowed set and that match any keyword (case-insensitive).
// Returns the top maxResults matches with content truncated to maxFileLines.
func RetrieveRelevantFiles(repoPath string, keywords []string, extensions []string) ([]CodeSnippet, error) {
	// Build a fast lookup set for allowed extensions.
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[strings.ToLower(e)] = true
	}
	type scoredFile struct {
		path  string
		score int
	}

	var candidates []scoredFile

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden directories (e.g. .git, .cache).
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !extSet[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Printf("[retriever] skipping %s: %v", path, readErr)
			return nil
		}

		lower := strings.ToLower(string(data))
		score := 0
		for _, kw := range keywords {
			score += strings.Count(lower, kw)
		}

		if score > 0 {
			candidates = append(candidates, scoredFile{path: path, score: score})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort descending by score (simple insertion sort — list is small).
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
		data, err := os.ReadFile(c.path)
		if err != nil {
			log.Printf("[retriever] re-read error for %s: %v", c.path, err)
			continue
		}
		content := truncateLines(string(data), maxFileLines)
		// Use a path relative to the repo root for cleaner display.
		relPath, relErr := filepath.Rel(repoPath, c.path)
		if relErr != nil {
			relPath = c.path
		}
		snippets = append(snippets, CodeSnippet{
			FilePath: relPath,
			Content:  content,
		})
		log.Printf("[retriever] matched file: %s (score=%d)", relPath, c.score)
	}

	return snippets, nil
}
