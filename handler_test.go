package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerWebhook_WrongPath(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir()}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerWebhook_WrongMethod(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir()}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerWebhook_WrongEventType(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir()}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{}`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandlerWebhook_InvalidPayload(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir()}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{not json}`))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerWebhook_NonOpenedAction(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir()}
	h := NewHandler(cfg)
	body := webhookJSON("closed", "org", "repo", 1, "title", "body")
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for non-opened action, got %d", w.Code)
	}
}

func TestHandlerWebhook_OpenedReturnsAccepted(t *testing.T) {
	cfg := &Config{GitHubToken: "t", OpenAIKey: "k", OpenAIBaseURL: "http://localhost", OpenAIModel: "m", RepoPath: t.TempDir(), WebhookActions: map[string]bool{"opened": true}}
	h := NewHandler(cfg)
	body := webhookJSON("opened", "org", "repo", 1, "title", "body")
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// Background goroutine fires but won't reach real GitHub/LLM — we just
	// assert the HTTP layer immediately returns 202.
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestHandlerWebhook_EditedWithForceTrigger(t *testing.T) {
	cfg := &Config{
		GitHubToken:    "t",
		OpenAIKey:      "k",
		OpenAIBaseURL:  "http://localhost",
		OpenAIModel:    "m",
		RepoPath:       t.TempDir(),
		WebhookActions: map[string]bool{"edited": true},
	}
	h := NewHandler(cfg)
	// Body contains the force-trigger phrase — should bypass significance check and
	// return 202 without needing to call the GitHub API to check for prior comments.
	body := webhookJSON("edited", "org", "repo", 1, "title", "some context\n\nproduct-agent:comment")
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 for force trigger, got %d", w.Code)
	}
}
