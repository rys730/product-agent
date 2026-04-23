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
	"time"
)

const (
	agentTimeout = 60 * time.Second
	maxRetries   = 1
)

// openAIRequest is the request body for the chat completions endpoint.
type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse is the partial response shape we care about.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// AgentClient holds the configuration for talking to the LLM API.
type AgentClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
	// Optional extra headers (e.g. HTTP-Referer and X-Title for OpenRouter).
	extraHeaders map[string]string
}

// NewAgentClient creates a ready-to-use AgentClient.
func NewAgentClient(apiKey, baseURL, model string) *AgentClient {
	return &AgentClient{
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		model:        model,
		http:         &http.Client{Timeout: agentTimeout},
		extraHeaders: map[string]string{},
	}
}

// WithHeaders returns a copy of the client with additional HTTP headers set.
// Use this for OpenRouter's HTTP-Referer / X-Title headers.
func (c *AgentClient) WithHeaders(headers map[string]string) *AgentClient {
	merged := make(map[string]string, len(c.extraHeaders)+len(headers))
	for k, v := range c.extraHeaders {
		merged[k] = v
	}
	for k, v := range headers {
		merged[k] = v
	}
	return &AgentClient{
		apiKey:       c.apiKey,
		baseURL:      c.baseURL,
		model:        c.model,
		http:         c.http,
		extraHeaders: merged,
	}
}

// RunAgent sends the prompt to the LLM and returns a parsed AgentOutput.
// It retries once on transient failure.
func (c *AgentClient) RunAgent(ctx context.Context, prompt string) (AgentOutput, error) {
	var (
		out AgentOutput
		err error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[agent] retrying (attempt %d)...", attempt+1)
			time.Sleep(2 * time.Second)
		}
		out, err = c.callLLM(ctx, prompt)
		if err == nil {
			return out, nil
		}
		log.Printf("[agent] attempt %d failed: %v", attempt+1, err)
	}
	return AgentOutput{}, fmt.Errorf("llm call failed after %d attempts: %w", maxRetries+1, err)
}

func (c *AgentClient) callLLM(ctx context.Context, prompt string) (AgentOutput, error) {
	reqBody := openAIRequest{
		Model: c.model,
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return AgentOutput{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return AgentOutput{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return AgentOutput{}, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return AgentOutput{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return AgentOutput{}, fmt.Errorf("llm api status %d: %s", resp.StatusCode, string(respBytes))
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(respBytes, &oaiResp); err != nil {
		return AgentOutput{}, fmt.Errorf("unmarshal openai response: %w", err)
	}
	if oaiResp.Error != nil {
		return AgentOutput{}, fmt.Errorf("openai error: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Choices) == 0 {
		return AgentOutput{}, fmt.Errorf("no choices in response")
	}

	raw := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	// Strip accidental markdown fences the model may add despite instructions.
	raw = stripMarkdownFence(raw)

	var out AgentOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return AgentOutput{}, fmt.Errorf("parse agent json output: %w\nraw content: %s", err, raw)
	}
	return out, nil
}

// stripMarkdownFence removes ```json ... ``` wrappers if present.
func stripMarkdownFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
