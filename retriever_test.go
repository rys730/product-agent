package main

import (
"os"
"path/filepath"
"strings"
"testing"
)

var goOnly = []string{".go"}

func makeTestRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRetrieveRelevantFiles_MatchesKeyword(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{
"handler.go":   "func HandleRequest(w http.ResponseWriter) {}",
"main.go":      "func main() { log.Println(\"start\") }",
"unrelated.go": "func foo() {}",
})
	snippets, err := RetrieveRelevantFiles(dir, []string{"handlerequest"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	if snippets[0].FilePath != "handler.go" {
		t.Errorf("expected handler.go, got %q", snippets[0].FilePath)
	}
}

func TestRetrieveRelevantFiles_Top5Only(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"} {
		files[name] = "func ratelimit() {}"
	}
	dir := makeTestRepo(t, files)
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) > maxResults {
		t.Errorf("expected at most %d results, got %d", maxResults, len(snippets))
	}
}

func TestRetrieveRelevantFiles_NoMatch(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{"main.go": "func main() {}"})
	snippets, err := RetrieveRelevantFiles(dir, []string{"zzznomatch"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 0 {
		t.Errorf("expected 0 results, got %d", len(snippets))
	}
}

func TestRetrieveRelevantFiles_SkipsNonAllowedExtensions(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{
"notes.txt": "ratelimit info here",
"main.go":   "func main() {}",
})
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 0 {
		t.Errorf("expected 0 matches, got %d", len(snippets))
	}
}

func TestRetrieveRelevantFiles_MultiLanguage(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{
"server.go":      "func ratelimit() {}",
"client.ts":      "function ratelimit() {}",
"helpers.py":     "def ratelimit(): pass",
"unrelated.java": "class Foo {}",
})
	exts := []string{".go", ".ts", ".py"}
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, exts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 3 {
		t.Errorf("expected 3 snippets (go+ts+py), got %d", len(snippets))
	}
	for _, s := range snippets {
		if strings.HasSuffix(s.FilePath, ".java") {
			t.Errorf("java file should have been excluded: %s", s.FilePath)
		}
	}
}

func TestRetrieveRelevantFiles_SkipsHiddenDirs(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{
".git/packed.go": "func ratelimit() {}",
"main.go":        "func main() {}",
})
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 0 {
		t.Errorf("expected hidden dir to be skipped, got %d matches", len(snippets))
	}
}

func TestRetrieveRelevantFiles_ContentTruncated(t *testing.T) {
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = "// line"
	}
	dir := makeTestRepo(t, map[string]string{
"big.go": strings.Join(lines, "\n") + "\nfunc ratelimit() {}",
})
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) == 0 {
		t.Fatal("expected 1 snippet")
	}
	lineCount := strings.Count(snippets[0].Content, "\n")
	if lineCount > maxFileLines+2 {
		t.Errorf("content not truncated: got %d lines", lineCount)
	}
	if !strings.Contains(snippets[0].Content, "truncated") {
		t.Errorf("expected truncation notice in content")
	}
}

func TestRetrieveRelevantFiles_RelativePaths(t *testing.T) {
	dir := makeTestRepo(t, map[string]string{
"sub/handler.go": "func ratelimit() {}",
})
	snippets, err := RetrieveRelevantFiles(dir, []string{"ratelimit"}, goOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) == 0 {
		t.Fatal("expected 1 snippet")
	}
	if filepath.IsAbs(snippets[0].FilePath) {
		t.Errorf("expected relative path, got %q", snippets[0].FilePath)
	}
}

// ── parseExtensions ───────────────────────────────────────────────────────────

func TestParseExtensions_NormalizesInput(t *testing.T) {
	exts := parseExtensions(".go, .TS, py, .RS")
	want := map[string]bool{".go": true, ".ts": true, ".py": true, ".rs": true}
	if len(exts) != len(want) {
		t.Fatalf("expected %d extensions, got %d: %v", len(want), len(exts), exts)
	}
	for _, e := range exts {
		if !want[e] {
			t.Errorf("unexpected extension: %q", e)
		}
	}
}

func TestParseExtensions_SkipsBlanks(t *testing.T) {
	exts := parseExtensions(".go,,,.ts,")
	if len(exts) != 2 {
		t.Errorf("expected 2, got %d: %v", len(exts), exts)
	}
}
