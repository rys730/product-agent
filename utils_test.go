package main

import (
	"os"
	"strings"
	"testing"
)

// ── ExtractKeywords ──────────────────────────────────────────────────────────

func TestExtractKeywords_Basic(t *testing.T) {
	kws := ExtractKeywords("Add rate limiting to the API handler")
	want := map[string]bool{"add": true, "rate": true, "limiting": true, "api": true, "handler": true}
	for _, k := range kws {
		if !want[k] {
			t.Errorf("unexpected keyword: %q", k)
		}
		delete(want, k)
	}
	for k := range want {
		t.Errorf("missing expected keyword: %q", k)
	}
}

func TestExtractKeywords_StopwordsRemoved(t *testing.T) {
	kws := ExtractKeywords("the and or but is it as be")
	if len(kws) != 0 {
		t.Errorf("expected no keywords, got %v", kws)
	}
}

func TestExtractKeywords_Deduplication(t *testing.T) {
	kws := ExtractKeywords("rate rate rate limiting limiting")
	if len(kws) != 2 {
		t.Errorf("expected 2 unique keywords, got %d: %v", len(kws), kws)
	}
}

func TestExtractKeywords_ShortWordsSkipped(t *testing.T) {
	kws := ExtractKeywords("go is ok so do")
	// "go" and "ok" are 2 chars; "is","so","do" are stopwords — all skipped
	if len(kws) != 0 {
		t.Errorf("expected 0 keywords, got %v", kws)
	}
}

func TestExtractKeywords_Punctuation(t *testing.T) {
	kws := ExtractKeywords("rate-limiting, authentication! webhook.")
	want := map[string]bool{"rate": true, "limiting": true, "authentication": true, "webhook": true}
	for _, k := range kws {
		if !want[k] {
			t.Errorf("unexpected keyword: %q", k)
		}
	}
}

func TestExtractKeywords_Empty(t *testing.T) {
	kws := ExtractKeywords("")
	if len(kws) != 0 {
		t.Errorf("expected empty slice, got %v", kws)
	}
}

// ── truncateLines ─────────────────────────────────────────────────────────────

func TestTruncateLines_UnderLimit(t *testing.T) {
	input := "line1\nline2\nline3"
	out := truncateLines(input, 10)
	if out != input {
		t.Errorf("expected unchanged content, got %q", out)
	}
}

func TestTruncateLines_ExactLimit(t *testing.T) {
	lines := "a\nb\nc"
	out := truncateLines(lines, 3)
	if out != lines {
		t.Errorf("expected unchanged content, got %q", out)
	}
}

func TestTruncateLines_OverLimit(t *testing.T) {
	input := "1\n2\n3\n4\n5"
	out := truncateLines(input, 3)
	if !strings.HasPrefix(out, "1\n2\n3") {
		t.Errorf("expected first 3 lines preserved, got %q", out)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation notice, got %q", out)
	}
}

// ── FormatComment ─────────────────────────────────────────────────────────────

func TestFormatComment_ContainsAllSections(t *testing.T) {
	out := AgentOutput{
		Summary:            "Do the thing",
		Requirements:       []string{"req1", "req2"},
		AffectedComponents: []string{"handler.go"},
		ProposedChanges:    []ProposedChange{{File: "main.go", Description: "add flag"}},
		EdgeCases:          []string{"nil input"},
		OpenQuestions:      []string{"which approach?"},
	}
	comment := FormatComment(out)

	sections := []string{
		"Summary", "Requirements", "Affected Components",
		"Proposed Changes", "Edge Cases", "Open Questions",
		"Do the thing", "req1", "handler.go", "main.go", "nil input", "which approach?",
	}
	for _, s := range sections {
		if !strings.Contains(comment, s) {
			t.Errorf("FormatComment missing %q", s)
		}
	}
}

func TestFormatComment_EmptySlices(t *testing.T) {
	out := AgentOutput{Summary: "minimal"}
	comment := FormatComment(out)
	if !strings.Contains(comment, "minimal") {
		t.Errorf("expected summary in comment")
	}
	// Should not panic on nil/empty slices.
}

// ── loadDotEnv ────────────────────────────────────────────────────────────────

func TestLoadDotEnv_LoadsValues(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), ".env")
	f.WriteString("TEST_DOTENV_FOO=bar\nTEST_DOTENV_BAZ=qux\n")
	f.Close()
	os.Unsetenv("TEST_DOTENV_FOO")
	os.Unsetenv("TEST_DOTENV_BAZ")

	loadDotEnv(f.Name())

	if got := os.Getenv("TEST_DOTENV_FOO"); got != "bar" {
		t.Errorf("FOO: got %q want %q", got, "bar")
	}
	if got := os.Getenv("TEST_DOTENV_BAZ"); got != "qux" {
		t.Errorf("BAZ: got %q want %q", got, "qux")
	}
}

func TestLoadDotEnv_DoesNotOverrideExisting(t *testing.T) {
	os.Setenv("TEST_DOTENV_EXISTING", "original")
	t.Cleanup(func() { os.Unsetenv("TEST_DOTENV_EXISTING") })

	f, _ := os.CreateTemp(t.TempDir(), ".env")
	f.WriteString("TEST_DOTENV_EXISTING=overwritten\n")
	f.Close()

	loadDotEnv(f.Name())

	if got := os.Getenv("TEST_DOTENV_EXISTING"); got != "original" {
		t.Errorf("expected %q, got %q", "original", got)
	}
}

func TestLoadDotEnv_SkipsCommentsAndBlanks(t *testing.T) {
	os.Unsetenv("TEST_DOTENV_COMMENT")
	f, _ := os.CreateTemp(t.TempDir(), ".env")
	f.WriteString("# this is a comment\n\nTEST_DOTENV_COMMENT=yes\n")
	f.Close()

	loadDotEnv(f.Name())

	if got := os.Getenv("TEST_DOTENV_COMMENT"); got != "yes" {
		t.Errorf("expected %q, got %q", "yes", got)
	}
}

func TestLoadDotEnv_StripsQuotes(t *testing.T) {
	os.Unsetenv("TEST_DOTENV_QUOTED1")
	os.Unsetenv("TEST_DOTENV_QUOTED2")
	f, _ := os.CreateTemp(t.TempDir(), ".env")
	f.WriteString("TEST_DOTENV_QUOTED1=\"hello world\"\nTEST_DOTENV_QUOTED2='single'\n")
	f.Close()

	loadDotEnv(f.Name())

	if got := os.Getenv("TEST_DOTENV_QUOTED1"); got != "hello world" {
		t.Errorf("double-quoted: got %q", got)
	}
	if got := os.Getenv("TEST_DOTENV_QUOTED2"); got != "single" {
		t.Errorf("single-quoted: got %q", got)
	}
}

func TestLoadDotEnv_MissingFileIsNoop(t *testing.T) {
	// Should not panic or log an error — just silently skip.
	loadDotEnv("/tmp/this_file_does_not_exist_product_agent.env")
}

// ── isSignificantEdit ─────────────────────────────────────────────────────────

func TestIsSignificantEdit_SignificantChange(t *testing.T) {
	old := "add logging to the server"
	new := "add logging to the server with structured json output rotation and alerting support"
	if !isSignificantEdit(old, new) {
		t.Error("expected significant edit (many new keywords)")
	}
}

func TestIsSignificantEdit_TrivialChange(t *testing.T) {
	old := "add logging to the server startup"
	new := "add logging to the server startup."  // just a punctuation fix
	if isSignificantEdit(old, new) {
		t.Error("expected insignificant edit (no new keywords)")
	}
}

func TestIsSignificantEdit_Typofix(t *testing.T) {
	old := "implement rate limitting on the api handler"
	new := "implement rate limiting on the api handler"  // typo fix
	if isSignificantEdit(old, new) {
		t.Error("expected insignificant edit (typo fix introduces < 3 new keywords)")
	}
}

func TestIsSignificantEdit_NewRequirements(t *testing.T) {
	old := "add authentication to the api"
	new := "add authentication to the api with jwt tokens refresh expiry blacklist redis storage"
	if !isSignificantEdit(old, new) {
		t.Error("expected significant edit (new technical requirements added)")
	}
}

func TestIsSignificantEdit_EmptyOld(t *testing.T) {
	// Simulates "opened" with no body, then edited with full content.
	if !isSignificantEdit("", "implement rate limiting with token bucket algorithm per ip address") {
		t.Error("expected significant edit when old text is empty")
	}
}
