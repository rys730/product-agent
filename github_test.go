package main

import (
"encoding/json"
"testing"
)

func webhookJSON(action, owner, repo string, number int, title, body string) []byte {
	p := webhookPayload{}
	p.Action = action
	p.Repository.Name = repo
	p.Repository.Owner.Login = owner
	p.Issue.Number = number
	p.Issue.Title = title
	p.Issue.Body = body
	b, _ := json.Marshal(p)
	return b
}

func TestParseWebhookPayload_Valid(t *testing.T) {
	raw := webhookJSON("opened", "my-org", "my-repo", 7, "Fix bug", "some body")
	issue, err := ParseWebhookPayload(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Owner != "my-org" {
		t.Errorf("owner: got %q want %q", issue.Owner, "my-org")
	}
	if issue.Repo != "my-repo" {
		t.Errorf("repo: got %q want %q", issue.Repo, "my-repo")
	}
	if issue.Number != 7 {
		t.Errorf("number: got %d want 7", issue.Number)
	}
	if issue.Title != "Fix bug" {
		t.Errorf("title: got %q", issue.Title)
	}
	if issue.Body != "some body" {
		t.Errorf("body: got %q", issue.Body)
	}
}

func TestParseWebhookPayload_MissingAction(t *testing.T) {
	raw := webhookJSON("", "org", "repo", 1, "title", "body")
	_, err := ParseWebhookPayload(raw)
	if err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestParseWebhookPayload_MissingRepo(t *testing.T) {
	raw := webhookJSON("opened", "", "", 1, "title", "body")
	_, err := ParseWebhookPayload(raw)
	if err == nil {
		t.Fatal("expected error for missing repo info")
	}
}

func TestParseWebhookPayload_MissingIssueNumber(t *testing.T) {
	raw := webhookJSON("opened", "org", "repo", 0, "title", "body")
	_, err := ParseWebhookPayload(raw)
	if err == nil {
		t.Fatal("expected error for missing issue number")
	}
}

func TestParseWebhookPayload_InvalidJSON(t *testing.T) {
	_, err := ParseWebhookPayload([]byte("{not json}"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
