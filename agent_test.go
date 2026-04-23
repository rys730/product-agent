package main

import (
"context"
"encoding/json"
"net/http"
"net/http/httptest"
"strings"
"testing"
)

func makeOpenAIResponse(t *testing.T, content string) []byte {
	t.Helper()
	type msg struct {
		Content string `json:"content"`
	}
	type choice struct {
		Message msg `json:"message"`
	}
	resp := struct {
		Choices []choice `json:"choices"`
	}{
		Choices: []choice{{Message: msg{Content: content}}},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestRunAgent_Success(t *testing.T) {
	payload := AgentOutput{
		Summary:      "test summary",
		Requirements: []string{"req1"},
	}
	raw, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
		w.Write(makeOpenAIResponse(t, string(raw)))
	}))
	defer srv.Close()

	client := NewAgentClient("test-key", srv.URL, "gpt-test")
	out, err := client.RunAgent(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Summary != "test summary" {
		t.Errorf("summary: got %q want %q", out.Summary, "test summary")
	}
	if len(out.Requirements) != 1 || out.Requirements[0] != "req1" {
		t.Errorf("requirements: got %v", out.Requirements)
	}
}

func TestRunAgent_StripsMarkdownFence(t *testing.T) {
	payload := AgentOutput{Summary: "fenced"}
	raw, _ := json.Marshal(payload)
	fenced := "```json\n" + string(raw) + "\n```"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Write(makeOpenAIResponse(t, fenced))
}))
	defer srv.Close()

	client := NewAgentClient("key", srv.URL, "m")
	out, err := client.RunAgent(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Summary != "fenced" {
		t.Errorf("expected summary 'fenced', got %q", out.Summary)
	}
}

func TestRunAgent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
http.Error(w, "internal error", http.StatusInternalServerError)
}))
	defer srv.Close()

	client := NewAgentClient("key", srv.URL, "m")
	// maxRetries=1 so two attempts total — keep test fast
	_, err := client.RunAgent(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestRunAgent_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Write(makeOpenAIResponse(t, "not json at all"))
}))
	defer srv.Close()

	client := NewAgentClient("key", srv.URL, "m")
	_, err := client.RunAgent(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error on invalid JSON output")
	}
}

func TestRunAgent_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Write([]byte(`{"choices":[]}`))
}))
	defer srv.Close()

	client := NewAgentClient("key", srv.URL, "m")
	_, err := client.RunAgent(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error when choices is empty")
	}
}

func TestRunAgent_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
gotAuth = r.Header.Get("Authorization")
payload, _ := json.Marshal(AgentOutput{Summary: "ok"})
w.Write(makeOpenAIResponse(t, string(payload)))
}))
	defer srv.Close()

	client := NewAgentClient("my-secret-key", srv.URL, "m")
	client.RunAgent(context.Background(), "prompt")

	if !strings.Contains(gotAuth, "my-secret-key") {
		t.Errorf("expected auth header to contain key, got %q", gotAuth)
	}
}

// ── stripMarkdownFence ────────────────────────────────────────────────────────

func TestStripMarkdownFence(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"```json\n{}\n```", "{}"},
		{"```\n{}\n```", "{}"},
		{"{}", "{}"},
		{"  {\"a\":1}  ", `{"a":1}`},
	}
	for _, c := range cases {
		got := stripMarkdownFence(c.input)
		if got != c.want {
			t.Errorf("stripMarkdownFence(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
