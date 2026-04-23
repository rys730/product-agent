package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BuildCodebaseIndex generates a Markdown summary of exported functions/methods
// across all files in repoPath that match the allowed extensions.
// Go files are parsed with go/ast for accurate signatures and doc comments.
// Other languages are scanned with language-specific regexes.
// The result is injected into the LLM prompt as structural context.
func BuildCodebaseIndex(repoPath string, extensions []string) string {
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[strings.ToLower(e)] = true
	}

	var sb strings.Builder

	_ = filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !extSet[ext] {
			return nil
		}

		rel, _ := filepath.Rel(repoPath, path)

		var entries []string
		switch ext {
		case ".go":
			entries = indexGoFile(path)
		case ".ts", ".js":
			entries = indexWithRegex(path, reFuncTS)
		case ".py":
			entries = indexWithRegex(path, reFuncPython)
		case ".rb":
			entries = indexWithRegex(path, reFuncRuby)
		case ".java", ".cs":
			entries = indexWithRegex(path, reFuncJava)
		case ".rs":
			entries = indexWithRegex(path, reFuncRust)
		}

		if len(entries) > 0 {
			fmt.Fprintf(&sb, "## %s\n", rel)
			for _, e := range entries {
				fmt.Fprintf(&sb, "- %s\n", e)
			}
			sb.WriteByte('\n')
		}
		return nil
	})

	return sb.String()
}

// BuildCodebaseIndexFromSnippets generates a lightweight index from already-fetched
// code snippets (used in github mode to avoid extra API calls).
func BuildCodebaseIndexFromSnippets(snippets []CodeSnippet) string {
	var sb strings.Builder
	for _, s := range snippets {
		ext := strings.ToLower(filepath.Ext(s.FilePath))
		var entries []string
		switch ext {
		case ".go":
			entries = indexGoSource(s.FilePath, s.Content)
		case ".ts", ".js":
			entries = indexSourceWithRegex(s.Content, reFuncTS)
		case ".py":
			entries = indexSourceWithRegex(s.Content, reFuncPython)
		case ".rb":
			entries = indexSourceWithRegex(s.Content, reFuncRuby)
		case ".java", ".cs":
			entries = indexSourceWithRegex(s.Content, reFuncJava)
		case ".rs":
			entries = indexSourceWithRegex(s.Content, reFuncRust)
		}
		if len(entries) > 0 {
			fmt.Fprintf(&sb, "## %s\n", s.FilePath)
			for _, e := range entries {
				fmt.Fprintf(&sb, "- %s\n", e)
			}
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// indexGoFile parses a Go source file from disk and returns function/method entries.
func indexGoFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[indexer] skipping %s: %v", path, err)
		return nil
	}
	return indexGoSource(path, string(data))
}

// indexGoSource parses Go source from a string and returns function/method entries.
func indexGoSource(filename, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		// Fall back to regex on parse failure.
		return indexSourceWithRegex(src, reFuncGo)
	}

	// Use go/doc to associate doc comments with declarations.
	pkg, _ := doc.NewFromFiles(fset, []*ast.File{f}, "")

	var entries []string

	// Top-level functions.
	for _, fn := range pkg.Funcs {
		entries = append(entries, formatGoFunc(fn.Name, fn.Decl, fn.Doc))
	}

	// Methods on types.
	for _, t := range pkg.Types {
		for _, fn := range t.Methods {
			entries = append(entries, formatGoFunc(t.Name+"."+fn.Name, fn.Decl, fn.Doc))
		}
		// Factory functions (Funcs on types).
		for _, fn := range t.Funcs {
			entries = append(entries, formatGoFunc(fn.Name, fn.Decl, fn.Doc))
		}
	}

	return entries
}

// formatGoFunc formats a single Go function entry with optional doc comment.
func formatGoFunc(name string, decl *ast.FuncDecl, docComment string) string {
	sig := goFuncSignature(name, decl)
	docComment = strings.TrimSpace(docComment)
	// Collapse multi-line doc to single line.
	docComment = strings.ReplaceAll(docComment, "\n", " ")
	if docComment != "" {
		return fmt.Sprintf("`%s` — %s", sig, docComment)
	}
	return fmt.Sprintf("`%s`", sig)
}

// goFuncSignature builds a compact "name(params) returns" string.
func goFuncSignature(name string, decl *ast.FuncDecl) string {
	if decl == nil {
		return name + "(...)"
	}
	var params []string
	if decl.Type.Params != nil {
		for _, field := range decl.Type.Params.List {
			typeStr := typeString(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeStr)
			} else {
				for _, n := range field.Names {
					params = append(params, n.Name+" "+typeStr)
				}
			}
		}
	}
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(params, ", "))
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		var results []string
		for _, field := range decl.Type.Results.List {
			results = append(results, typeString(field.Type))
		}
		if len(results) == 1 {
			sig += " " + results[0]
		} else {
			sig += " (" + strings.Join(results, ", ") + ")"
		}
	}
	return sig
}

// typeString renders an ast.Expr as a compact type string.
func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	default:
		return "..."
	}
}

// Regex patterns for non-Go languages — capture the function signature line.
var (
	reFuncGo     = regexp.MustCompile(`(?m)^func\s+\(?[^)]*\)?\s*(\w+)\s*\([^)]*\)[^{]*`)
	reFuncTS     = regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+\w+\s*\([^)]*\)[^{]*`)
	reFuncPython = regexp.MustCompile(`(?m)^(?:    )?def\s+\w+\s*\([^)]*\)\s*(?:->[^:]+)?:`)
	reFuncRuby   = regexp.MustCompile(`(?m)^\s*def\s+\w+(?:\([^)]*\))?`)
	reFuncJava   = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|static|\s)+[\w<>\[\]]+\s+\w+\s*\([^)]*\)\s*(?:throws\s+\w+)?\s*\{`)
	reFuncRust   = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?fn\s+\w+\s*(?:<[^>]*>)?\s*\([^)]*\)(?:\s*->[^{]+)?`)
)

// indexWithRegex reads a file and extracts function signatures via regex.
func indexWithRegex(path string, re *regexp.Regexp) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return indexSourceWithRegex(string(data), re)
}

// indexSourceWithRegex extracts function signatures from source string via regex.
func indexSourceWithRegex(src string, re *regexp.Regexp) []string {
	matches := re.FindAllString(src, -1)
	entries := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimSpace(m)
		// Strip trailing brace if present.
		m = strings.TrimSuffix(m, "{")
		m = strings.TrimSpace(m)
		entries = append(entries, "`"+m+"`")
	}
	return entries
}
