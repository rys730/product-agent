package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	GitHubToken   string
	OpenAIKey     string
	OpenAIBaseURL string
	OpenAIModel   string
	RepoPath      string
	Port          string
	// Comma-separated list of file extensions to scan, e.g. ".go,.ts,.py"
	RepoExtensions []string
	// RetrieveMode controls which retriever backend is used.
	// "local" (default) scans REPO_PATH on disk.
	// "github" uses the GitHub Trees + Contents API — no local clone needed.
	RetrieveMode string
	// RepoBranch is the branch to scan in GitHub API mode.
	// Leave empty to use the repo's default branch automatically.
	RepoBranch string
	// WebhookActions is the set of issue actions that trigger the pipeline.
	// e.g. {"opened": true, "edited": true}
	WebhookActions map[string]bool
	// OpenRouter optional headers
	OpenRouterReferer string
	OpenRouterTitle   string
}

// loadConfig reads required env vars and returns a Config.
// It returns an error if any required variable is missing.
func loadConfig() (*Config, error) {
	cfg := &Config{
		GitHubToken:       os.Getenv("GITHUB_TOKEN"),
		OpenAIKey:         os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:     getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:       getEnvOrDefault("OPENAI_MODEL", "gpt-4o"),
		RepoPath:          getEnvOrDefault("REPO_PATH", "."),
		Port:              getEnvOrDefault("PORT", "8080"),
		RepoExtensions:    parseExtensions(getEnvOrDefault("REPO_EXTENSIONS", ".go,.ts,.js,.py,.java,.rs,.rb,.cs,.cpp,.c")),
		RetrieveMode:      getEnvOrDefault("RETRIEVE_MODE", "local"),
		RepoBranch:        os.Getenv("REPO_BRANCH"),
		WebhookActions:    parseActions(getEnvOrDefault("WEBHOOK_ACTIONS", "opened,edited")),
		OpenRouterReferer: os.Getenv("OPENROUTER_REFERER"),
		OpenRouterTitle:   getEnvOrDefault("OPENROUTER_TITLE", "product-agent"),
	}

	missing := []string{}
	if cfg.GitHubToken == "" {
		missing = append(missing, "GITHUB_TOKEN")
	}
	if cfg.OpenAIKey == "" {
		missing = append(missing, "OPENAI_API_KEY")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}
	return cfg, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// jsonUnmarshal is a thin wrapper so handler.go can call it without importing encoding/json directly.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[product-agent] ")

	loadDotEnv(".env")

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	handler := NewHandler(cfg)

	addr := ":" + cfg.Port
	log.Printf("starting server on %s", addr)
	log.Printf("retrieve mode: %s", cfg.RetrieveMode)
	log.Printf("webhook actions: %v", cfg.WebhookActions)
	log.Printf("repo path: %s", cfg.RepoPath)
	log.Printf("model: %s", cfg.OpenAIModel)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
