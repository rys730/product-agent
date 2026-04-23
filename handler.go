package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Handler holds all dependencies needed to process webhook events.
type Handler struct {
	cfg       *Config
	github    *GitHubClient
	agent     *AgentClient
	retriever Retriever
}

// NewHandler constructs a Handler from the given config.
func NewHandler(cfg *Config) *Handler {
	agent := NewAgentClient(cfg.OpenAIKey, cfg.OpenAIBaseURL, cfg.OpenAIModel)

	// Attach OpenRouter-specific headers when a referer is configured.
	if cfg.OpenRouterReferer != "" {
		agent = agent.WithHeaders(map[string]string{
			"HTTP-Referer": cfg.OpenRouterReferer,
			"X-Title":      cfg.OpenRouterTitle,
		})
	}

	// Select retriever backend based on RETRIEVE_MODE.
	var retriever Retriever
	switch cfg.RetrieveMode {
	case "github":
		retriever = NewGitHubRetriever(cfg.GitHubToken, cfg.RepoExtensions)
		log.Printf("[handler] using GitHub API retriever")
	default:
		retriever = NewLocalRetriever(cfg.RepoPath, cfg.RepoExtensions)
		log.Printf("[handler] using local retriever (path: %s)", cfg.RepoPath)
	}

	return &Handler{
		cfg:       cfg,
		github:    NewGitHubClient(cfg.GitHubToken),
		agent:     agent,
		retriever: retriever,
	}
}

// ServeHTTP routes incoming requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/webhook" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	h.handleWebhook(w, r)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	log.Printf("[handler] request_id=%s received webhook", requestID)

	// Only process "issues" events.
	event := r.Header.Get("X-GitHub-Event")
	if event != "issues" {
		log.Printf("[handler] request_id=%s ignoring event type: %s", requestID, event)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		log.Printf("[handler] request_id=%s read body error: %v", requestID, err)
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	issue, err := ParseWebhookPayload(body)
	if err != nil {
		log.Printf("[handler] request_id=%s parse payload error: %v", requestID, err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Only act on configured actions (default: "opened").
	action := extractAction(body)
	if !h.cfg.WebhookActions[action] {
		log.Printf("[handler] request_id=%s ignoring action: %s", requestID, action)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// For "edited" actions, skip if the change is not significant.
	if action == "edited" {
		oldText := parseOldText(body)
		newText := issue.Title + " " + issue.Body
		if !isSignificantEdit(oldText, newText) {
			log.Printf("[handler] request_id=%s edit not significant, skipping", requestID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		log.Printf("[handler] request_id=%s significant edit detected, processing", requestID)
	}

	log.Printf("[handler] request_id=%s processing issue %s/%s#%d: %q",
		requestID, issue.Owner, issue.Repo, issue.Number, issue.Title)

	// Run the full pipeline in a background goroutine so we can ACK GitHub quickly.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := h.process(ctx, requestID, issue); err != nil {
			log.Printf("[handler] request_id=%s pipeline error: %v", requestID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, `{"status":"accepted"}`)
}

// process runs the full agent pipeline for a single issue.
func (h *Handler) process(ctx context.Context, requestID string, issue GitHubIssue) error {
	// 1. Extract keywords from the issue text.
	log.Printf("[pipeline] request_id=%s extracting keywords", requestID)
	keywords := ExtractKeywords(issue.Title + " " + issue.Body)
	log.Printf("[pipeline] request_id=%s keywords: %v", requestID, keywords)

	// 2. Retrieve relevant code files.
	log.Printf("[pipeline] request_id=%s retrieving files (mode=%s)", requestID, h.cfg.RetrieveMode)
	snippets, err := h.retriever.Retrieve(ctx, issue, keywords)
	if err != nil {
		return fmt.Errorf("retrieve files: %w", err)
	}
	log.Printf("[pipeline] request_id=%s retrieved %d file(s)", requestID, len(snippets))

	// 3. Build the LLM prompt.
	log.Printf("[pipeline] request_id=%s building prompt", requestID)
	prompt := BuildPrompt(issue, snippets)

	// 4. Call the LLM.
	log.Printf("[pipeline] request_id=%s calling LLM", requestID)
	output, err := h.agent.RunAgent(ctx, prompt)
	if err != nil {
		return fmt.Errorf("run agent: %w", err)
	}
	log.Printf("[pipeline] request_id=%s LLM response received", requestID)

	// 5. Format into a GitHub Markdown comment.
	comment := FormatComment(output)

	// 6. Post the comment.
	log.Printf("[pipeline] request_id=%s posting comment", requestID)
	if err := h.github.PostComment(ctx, issue, comment); err != nil {
		return fmt.Errorf("post comment: %w", err)
	}

	log.Printf("[pipeline] request_id=%s done", requestID)
	return nil
}

// extractAction is a fast path to read the "action" field without full unmarshalling.
func extractAction(body []byte) string {
	var partial struct {
		Action string `json:"action"`
	}
	_ = jsonUnmarshal(body, &partial)
	return partial.Action
}
