# Lathe Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bootstrap the lathe repo into a working system: a Go CLI (`lathe`) for storing, listing, and serving tutorials, plus two Claude Code skills (`/lathe` for generation and an embedded verify skill) that together produce verified, hands-on technical tutorials.

**Architecture:** Skills generate markdown tutorials and call `lathe store --verify`, which copies files to `~/.lathe/tutorials/`, writes metadata, and spawns a detached `claude` subprocess to validate the tutorial in a temp directory. `lathe serve` starts a local web server that renders tutorials with sidebar navigation and live verification status badges.

**Tech Stack:** Go 1.22, cobra, goldmark, goldmark-highlighting (chroma), net/http, html/template, embed

---

## File Structure

```
lathe/
├── main.go
├── cmd/
│   ├── root.go
│   ├── store.go
│   ├── serve.go
│   ├── list.go
│   └── open.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── store/
│   │   ├── metadata.go
│   │   ├── metadata_test.go
│   │   ├── store.go
│   │   └── store_test.go
│   ├── serve/
│   │   ├── server.go
│   │   ├── server_test.go
│   │   ├── renderer.go
│   │   ├── renderer_test.go
│   │   ├── layout.html
│   │   └── list.html
│   └── verify/
│       ├── verify.go
│       ├── verify_test.go
│       └── skills/
│           └── lathe-verify.md
├── .claude/
│   └── skills/
│       └── lathe/
│           └── lathe.md
├── go.mod
└── go.sum
```

**Key boundaries:**
- `internal/config` — one exported function, no imports from internal
- `internal/store` — Tutorial model + storage, imports config and verify
- `internal/serve` — HTTP server + markdown renderer, imports config and store
- `internal/verify` — subprocess spawning only, no internal imports
- `cmd/` — cobra commands, wire together internal packages
- The verify skill lives in `internal/verify/skills/` (embedded into the binary); it is never user-invoked so it does not live in `.claude/skills/`

---

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /Users/devenjarvis/Code/lathe
go mod init github.com/devenjarvis/lathe
```

Expected: `go.mod` created with `module github.com/devenjarvis/lathe` and `go 1.22` (or higher).

- [ ] **Step 2: Install dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/yuin/goldmark@latest
go get github.com/yuin/goldmark-highlighting/v2@latest
go mod tidy
```

Expected: `go.sum` created, no errors.

- [ ] **Step 3: Write `cmd/root.go`**

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lathe",
	Short: "Generate and manage hands-on technical tutorials",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Write `main.go`**

```go
package main

import "github.com/devenjarvis/lathe/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 5: Verify it builds**

```bash
go build ./...
```

Expected: no output, no errors. Binary not yet installed — we're just checking compilation.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum main.go cmd/root.go
git commit -m "feat: scaffold Go CLI with cobra"
```

---

### Task 2: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/devenjarvis/lathe/internal/config"
)

func TestTutorialsDir(t *testing.T) {
	dir, err := config.TutorialsDir()
	if err != nil {
		t.Fatalf("TutorialsDir() error = %v", err)
	}
	if !strings.HasSuffix(dir, ".lathe/tutorials") {
		t.Errorf("TutorialsDir() = %q, want path ending in .lathe/tutorials", dir)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("TutorialsDir() did not create directory at %q", dir)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```

Expected: `FAIL — cannot find package` or compilation error (config.go doesn't exist yet).

- [ ] **Step 3: Write the implementation**

```go
// internal/config/config.go
package config

import (
	"os"
	"path/filepath"
)

func TutorialsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lathe", "tutorials")
	return dir, os.MkdirAll(dir, 0755)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -v
```

Expected: `PASS — TestTutorialsDir`

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with TutorialsDir"
```

---

### Task 3: Metadata Model

**Files:**
- Create: `internal/store/metadata.go`
- Create: `internal/store/metadata_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/store/metadata_test.go
package store_test

import (
	"testing"
	"time"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestWriteReadMetadata(t *testing.T) {
	dir := t.TempDir()
	original := &store.Tutorial{
		Slug:    "test-tutorial",
		Title:   "Test Tutorial",
		Topic:   "test tutorial",
		Created: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Status:  store.StatusVerified,
		Series:  false,
	}

	if err := store.WriteMetadata(dir, original); err != nil {
		t.Fatalf("WriteMetadata() error = %v", err)
	}

	got, err := store.ReadMetadata(dir)
	if err != nil {
		t.Fatalf("ReadMetadata() error = %v", err)
	}
	if got.Slug != original.Slug {
		t.Errorf("Slug = %q, want %q", got.Slug, original.Slug)
	}
	if got.Status != original.Status {
		t.Errorf("Status = %q, want %q", got.Status, original.Status)
	}
}

func TestReadMetadataNotFound(t *testing.T) {
	_, err := store.ReadMetadata("/nonexistent/path/abc123")
	if err == nil {
		t.Error("ReadMetadata() expected error for missing file, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... 2>&1 | head -5
```

Expected: compilation error — package `store` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

```go
// internal/store/metadata.go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusVerifying Status = "verifying"
	StatusVerified  Status = "verified"
	StatusFailed    Status = "failed"
)

type Tutorial struct {
	Slug    string    `json:"slug"`
	Title   string    `json:"title"`
	Topic   string    `json:"topic"`
	Created time.Time `json:"created"`
	Status  Status    `json:"status"`
	Series  bool      `json:"series"`
	Parts   []string  `json:"parts,omitempty"`
}

type VerifyResult struct {
	Status     Status `json:"status"`
	Part       string `json:"part,omitempty"`
	FailedStep int    `json:"failed_step,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
}

func ReadMetadata(tutorialDir string) (*Tutorial, error) {
	data, err := os.ReadFile(filepath.Join(tutorialDir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var t Tutorial
	return &t, json.Unmarshal(data, &t)
}

func WriteMetadata(tutorialDir string, t *Tutorial) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tutorialDir, "metadata.json"), data, 0644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/store/... -v -run TestWrite
go test ./internal/store/... -v -run TestRead
```

Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/metadata.go internal/store/metadata_test.go
git commit -m "feat: add Tutorial metadata model with read/write"
```

---

### Task 4: Store Package

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/store/store_test.go
package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestStoreSingleTutorial(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "index.md"), []byte("# Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// Override home so we don't pollute the real ~/.lathe
	t.Setenv("HOME", t.TempDir())

	tut, err := store.Store(src, false)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if tut.Series {
		t.Error("Store() Series = true, want false for single tutorial")
	}
	if tut.Status != store.StatusVerified {
		t.Errorf("Store() Status = %q, want %q", tut.Status, store.StatusVerified)
	}
}

func TestStoreSeriesTutorial(t *testing.T) {
	src := t.TempDir()
	for _, name := range []string{"part-01.md", "part-02.md", "part-03.md"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte("# Part"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", t.TempDir())

	tut, err := store.Store(src, false)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if !tut.Series {
		t.Error("Store() Series = false, want true for series")
	}
	if len(tut.Parts) != 3 {
		t.Errorf("Store() Parts = %v, want 3 parts", tut.Parts)
	}
}

func TestStoreVerifyingStatus(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "index.md"), []byte("# Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	// withVerify=true would try to spawn claude; we skip that by not passing it.
	// This test uses false — verify the status is "verified" (default) when not verifying.
	tut, err := store.Store(src, false)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if tut.Status != store.StatusVerified {
		t.Errorf("Store() Status = %q, want %q", tut.Status, store.StatusVerified)
	}
}

func TestSlugToTitle(t *testing.T) {
	cases := []struct {
		slug  string
		title string
	}{
		{"digital-synth-zig", "Digital Synth Zig"},
		{"database-from-scratch-go", "Database From Scratch Go"},
		{"hello", "Hello"},
	}
	for _, c := range cases {
		got := store.SlugToTitle(c.slug)
		if got != c.title {
			t.Errorf("SlugToTitle(%q) = %q, want %q", c.slug, got, c.title)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -run TestStore 2>&1 | head -5
```

Expected: compilation error — `store.Store`, `store.SlugToTitle` not defined.

- [ ] **Step 3: Write the implementation**

```go
// internal/store/store.go
package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/verify"
)

func Store(srcPath string, withVerify bool) (*Tutorial, error) {
	slug := filepath.Base(strings.TrimSuffix(srcPath, string(filepath.Separator)))

	tutorialsDir, err := config.TutorialsDir()
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(tutorialsDir, slug)
	if err := copyDir(srcPath, destDir); err != nil {
		return nil, fmt.Errorf("copy tutorial: %w", err)
	}

	parts, series := detectParts(destDir)
	status := StatusVerified
	if withVerify {
		status = StatusVerifying
	}

	t := &Tutorial{
		Slug:    slug,
		Title:   SlugToTitle(slug),
		Topic:   slug,
		Created: time.Now().UTC(),
		Status:  status,
		Series:  series,
		Parts:   parts,
	}

	if err := WriteMetadata(destDir, t); err != nil {
		return nil, err
	}

	if withVerify {
		if err := verify.SpawnVerifier(slug, destDir); err != nil {
			return nil, fmt.Errorf("spawn verifier: %w", err)
		}
	}

	return t, nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func detectParts(dir string) ([]string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var parts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "part-") && strings.HasSuffix(e.Name(), ".md") {
			parts = append(parts, e.Name())
		}
	}
	sort.Strings(parts)
	return parts, len(parts) > 0
}

func SlugToTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
```

- [ ] **Step 4: Write a stub verify package so it compiles**

`internal/store/store.go` imports `internal/verify` which doesn't exist yet. Create a stub:

```go
// internal/verify/verify.go
package verify

func SpawnVerifier(slug, tutorialDir string) error {
	return nil // stub — implemented in Task 11
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/store/... -v
```

Expected: all four tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go internal/verify/verify.go
git commit -m "feat: add store package with Store(), detectParts(), SlugToTitle()"
```

---

### Task 5: `lathe store` Command

**Files:**
- Create: `cmd/store.go`
- Modify: `cmd/root.go` (add subcommand)

- [ ] **Step 1: Write `cmd/store.go`**

```go
// cmd/store.go
package cmd

import (
	"fmt"

	"github.com/devenjarvis/lathe/internal/store"
	"github.com/spf13/cobra"
)

var withVerify bool

var storeCmd = &cobra.Command{
	Use:   "store <path>",
	Short: "Save a tutorial directory to ~/.lathe/tutorials/",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tut, err := store.Store(args[0], withVerify)
		if err != nil {
			return err
		}
		fmt.Printf("Stored: %s (%s)\n", tut.Title, tut.Status)
		if withVerify {
			fmt.Println("Verification running in background. Run `lathe serve` to check status.")
		}
		return nil
	},
}

func init() {
	storeCmd.Flags().BoolVar(&withVerify, "verify", false, "spawn background verification after storing")
	rootCmd.AddCommand(storeCmd)
}
```

- [ ] **Step 2: Build and smoke test manually**

```bash
go build -o /tmp/lathe .
mkdir -p /tmp/lathe-hello-test
echo "# Hello" > /tmp/lathe-hello-test/index.md
/tmp/lathe store /tmp/lathe-hello-test
```

Expected output: `Stored: Hello Test (verified)`

```bash
cat ~/.lathe/tutorials/lathe-hello-test/metadata.json
```

Expected: JSON with slug, title, status "verified".

- [ ] **Step 3: Commit**

```bash
git add cmd/store.go
git commit -m "feat: add lathe store command"
```

---

### Task 6: `lathe list` Command

**Files:**
- Create: `cmd/list.go`

- [ ] **Step 1: Write `cmd/list.go`**

```go
// cmd/list.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored tutorials",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := config.TutorialsDir()
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No tutorials yet. Run /lathe in Claude Code to generate one.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SLUG\tTITLE\tSTATUS\tPARTS")
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			tut, err := store.ReadMetadata(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			parts := "single"
			if tut.Series {
				parts = fmt.Sprintf("%d parts", len(tut.Parts))
			}
			badge := statusBadge(tut.Status)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tut.Slug, tut.Title, badge, parts)
		}
		return w.Flush()
	},
}

func statusBadge(s store.Status) string {
	switch s {
	case store.StatusVerified:
		return "✅ verified"
	case store.StatusVerifying:
		return "⏳ verifying"
	case store.StatusFailed:
		return "❌ failed"
	default:
		return string(s)
	}
}

func init() {
	rootCmd.AddCommand(listCmd)
}
```

- [ ] **Step 2: Build and smoke test**

```bash
go build -o /tmp/lathe .
/tmp/lathe list
```

Expected: table showing the `lathe-hello-test` tutorial stored in Task 5.

- [ ] **Step 3: Commit**

```bash
git add cmd/list.go
git commit -m "feat: add lathe list command"
```

---

### Task 7: Markdown Renderer

**Files:**
- Create: `internal/serve/renderer.go`
- Create: `internal/serve/renderer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/serve/renderer_test.go
package serve_test

import (
	"strings"
	"testing"

	"github.com/devenjarvis/lathe/internal/serve"
)

func TestRenderMarkdown(t *testing.T) {
	src := []byte("# Hello World\n\nThis is a `test`.\n\n```go\nfmt.Println(\"hello\")\n```")

	out, err := serve.RenderMarkdown(src)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}

	html := string(out)
	if !strings.Contains(html, "<h1>Hello World</h1>") {
		t.Errorf("RenderMarkdown() missing <h1>, got:\n%s", html)
	}
	if !strings.Contains(html, "<code>test</code>") {
		t.Errorf("RenderMarkdown() missing inline <code>, got:\n%s", html)
	}
	if !strings.Contains(html, "fmt.Println") {
		t.Errorf("RenderMarkdown() code block content missing, got:\n%s", html)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/serve/... -run TestRenderMarkdown 2>&1 | head -5
```

Expected: compilation error — package `serve` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

```go
// internal/serve/renderer.go
package serve

import (
	"bytes"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark"
)

func RenderMarkdown(src []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/serve/... -run TestRenderMarkdown -v
```

Expected: `PASS — TestRenderMarkdown`

- [ ] **Step 5: Commit**

```bash
git add internal/serve/renderer.go internal/serve/renderer_test.go
git commit -m "feat: add markdown renderer with goldmark + syntax highlighting"
```

---

### Task 8: HTML Templates

**Files:**
- Create: `internal/serve/layout.html`
- Create: `internal/serve/list.html`

- [ ] **Step 1: Write `internal/serve/layout.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}} — Lathe</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f9fafb;color:#111827;line-height:1.6}
    .layout{display:flex;min-height:100vh}
    nav{width:260px;background:#fff;border-right:1px solid #e5e7eb;padding:1.5rem;position:sticky;top:0;height:100vh;overflow-y:auto;flex-shrink:0}
    nav h2{font-size:.9rem;font-weight:600;color:#374151;margin-bottom:.5rem;line-height:1.4}
    .badge{display:inline-block;font-size:.7rem;padding:.2rem .5rem;border-radius:4px;margin-bottom:1.25rem}
    .badge.verified{background:#d1fae5;color:#065f46}
    .badge.verifying{background:#fef3c7;color:#92400e}
    .badge.failed{background:#fee2e2;color:#991b1b}
    nav ul{list-style:none}
    nav li{margin:.2rem 0}
    nav a{display:block;font-size:.85rem;color:#4b5563;text-decoration:none;padding:.35rem .6rem;border-radius:5px}
    nav a:hover{background:#f3f4f6;color:#111827}
    nav a.active{background:#eff6ff;color:#1d4ed8;font-weight:500}
    main{flex:1;max-width:760px;padding:3rem 2.5rem}
    h1{font-size:1.9rem;margin-bottom:1.5rem;color:#111827}
    h2{font-size:1.4rem;margin:2rem 0 .75rem;color:#1f2937}
    h3{font-size:1.15rem;margin:1.75rem 0 .6rem;color:#374151}
    p{margin-bottom:1rem;color:#374151}
    code{font-family:'JetBrains Mono','Fira Code',monospace;font-size:.85em;background:#f3f4f6;padding:.1rem .35rem;border-radius:3px;color:#1f2937}
    pre{border-radius:8px;margin:1.25rem 0;overflow-x:auto}
    pre code{background:none;padding:0;color:inherit;font-size:.875rem}
    ul,ol{margin:.75rem 0 .75rem 1.5rem;color:#374151}
    li{margin:.25rem 0}
    blockquote{border-left:3px solid #d1d5db;padding:.5rem 1rem;margin:1rem 0;color:#6b7280}
  </style>
</head>
<body>
<div class="layout">
  <nav>
    <h2>{{.Tutorial.Title}}</h2>
    {{if eq .Tutorial.Status "verified"}}<span class="badge verified">✅ Verified</span>
    {{else if eq .Tutorial.Status "verifying"}}<span class="badge verifying">⏳ Verifying…</span>
    {{else if eq .Tutorial.Status "failed"}}<span class="badge failed">❌ Failed</span>
    {{end}}
    {{if .Tutorial.Series}}
    <ul>
      {{range $i, $part := .Tutorial.Parts}}
      <li><a href="/{{$.Tutorial.Slug}}/{{$part}}"{{if eq $part $.CurrentPart}} class="active"{{end}}>Part {{add $i 1}}</a></li>
      {{end}}
    </ul>
    {{end}}
  </nav>
  <main>{{.Content}}</main>
</div>
</body>
</html>
```

- [ ] **Step 2: Write `internal/serve/list.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Lathe — Tutorials</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f9fafb;color:#111827;max-width:700px;margin:3rem auto;padding:0 1.5rem}
    h1{font-size:1.75rem;margin-bottom:.4rem}
    .subtitle{color:#6b7280;margin-bottom:2rem;font-size:.95rem}
    .tutorial{background:#fff;border:1px solid #e5e7eb;border-radius:8px;padding:1.1rem 1.5rem;margin:.75rem 0;display:flex;justify-content:space-between;align-items:center}
    .tutorial a{text-decoration:none;color:#1d4ed8;font-weight:500;font-size:1rem}
    .tutorial a:hover{text-decoration:underline}
    .meta{font-size:.8rem;color:#9ca3af;margin-top:.2rem}
    .badge{font-size:.7rem;padding:.2rem .5rem;border-radius:4px}
    .badge.verified{background:#d1fae5;color:#065f46}
    .badge.verifying{background:#fef3c7;color:#92400e}
    .badge.failed{background:#fee2e2;color:#991b1b}
    .empty{color:#9ca3af;text-align:center;padding:3rem;font-size:.95rem}
  </style>
</head>
<body>
<h1>Lathe</h1>
<p class="subtitle">Your generated tutorials</p>
{{if .Tutorials}}
{{range .Tutorials}}
<div class="tutorial">
  <div>
    <a href="/{{.Slug}}/">{{.Title}}</a>
    <div class="meta">{{if .Series}}{{len .Parts}} parts{{else}}single tutorial{{end}} · {{.Created.Format "Jan 2, 2006"}}</div>
  </div>
  {{if eq .Status "verified"}}<span class="badge verified">✅ Verified</span>
  {{else if eq .Status "verifying"}}<span class="badge verifying">⏳ Verifying…</span>
  {{else if eq .Status "failed"}}<span class="badge failed">❌ Failed</span>
  {{end}}
</div>
{{end}}
{{else}}
<div class="empty">No tutorials yet. Run <code>/lathe</code> in Claude Code to generate one.</div>
{{end}}
</body>
</html>
```

- [ ] **Step 3: Verify files are valid by building**

```bash
go build ./...
```

Expected: no errors (templates aren't referenced by Go code yet, so this is just a sanity check).

- [ ] **Step 4: Commit**

```bash
git add internal/serve/layout.html internal/serve/list.html
git commit -m "feat: add HTML templates for tutorial and list pages"
```

---

### Task 9: Web Server

**Files:**
- Create: `internal/serve/server.go`
- Create: `internal/serve/server_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/serve/server_test.go
package serve_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/lathe/internal/serve"
	"github.com/devenjarvis/lathe/internal/store"
)

func makeTestTutorial(t *testing.T, dir, slug string, series bool) string {
	t.Helper()
	tutDir := filepath.Join(dir, slug)
	if err := os.MkdirAll(tutDir, 0755); err != nil {
		t.Fatal(err)
	}
	tut := &store.Tutorial{
		Slug:    slug,
		Title:   "Test Tutorial",
		Status:  store.StatusVerified,
		Series:  series,
		Created: time.Now(),
	}
	if series {
		tut.Parts = []string{"part-01.md", "part-02.md"}
		for _, p := range tut.Parts {
			if err := os.WriteFile(filepath.Join(tutDir, p), []byte("# "+p), 0644); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		if err := os.WriteFile(filepath.Join(tutDir, "index.md"), []byte("# Index"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteMetadata(tutDir, tut); err != nil {
		t.Fatal(err)
	}
	return tutDir
}

func TestListPage(t *testing.T) {
	dir := t.TempDir()
	makeTestTutorial(t, dir, "test-tutorial", false)

	srv := serve.NewServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET / = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Test Tutorial") {
		t.Error("GET / response does not contain tutorial title")
	}
}

func TestTutorialPage(t *testing.T) {
	dir := t.TempDir()
	makeTestTutorial(t, dir, "test-tutorial", false)

	srv := serve.NewServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/test-tutorial/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test-tutorial/ = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Index") {
		t.Error("GET /test-tutorial/ response does not contain page content")
	}
}

func TestSeriesPartPage(t *testing.T) {
	dir := t.TempDir()
	makeTestTutorial(t, dir, "test-series", true)

	srv := serve.NewServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/test-series/part-01.md", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test-series/part-01.md = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestNotFound(t *testing.T) {
	dir := t.TempDir()
	srv := serve.NewServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent/ = %d, want %d", w.Code, http.StatusNotFound)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/serve/... -run TestListPage 2>&1 | head -5
```

Expected: compilation error — `serve.NewServer` not defined.

- [ ] **Step 3: Write the implementation**

```go
// internal/serve/server.go
package serve

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/devenjarvis/lathe/internal/store"
)

//go:embed layout.html list.html
var templateFS embed.FS

type Server struct {
	tutorialsDir string
	layoutTmpl   *template.Template
	listTmpl     *template.Template
}

func NewServer(tutorialsDir string) *Server {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	layoutTmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFS(templateFS, "layout.html"))
	listTmpl := template.Must(template.New("list.html").ParseFS(templateFS, "list.html"))
	return &Server{tutorialsDir: tutorialsDir, layoutTmpl: layoutTmpl, listTmpl: listTmpl}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleList)
	mux.HandleFunc("GET /{slug}/", s.handleTutorial)
	mux.HandleFunc("GET /{slug}/{part}", s.handlePart)
	return mux
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.tutorialsDir)
	if err != nil {
		http.Error(w, "could not read tutorials", http.StatusInternalServerError)
		return
	}
	var tutorials []*store.Tutorial
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tut, err := store.ReadMetadata(filepath.Join(s.tutorialsDir, e.Name()))
		if err != nil {
			continue
		}
		tutorials = append(tutorials, tut)
	}
	s.listTmpl.Execute(w, map[string]any{"Tutorials": tutorials})
}

func (s *Server) handleTutorial(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	tutDir := filepath.Join(s.tutorialsDir, slug)
	tut, err := store.ReadMetadata(tutDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if tut.Series && len(tut.Parts) > 0 {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s", slug, tut.Parts[0]), http.StatusFound)
		return
	}
	s.renderPart(w, tut, tutDir, "index.md")
}

func (s *Server) handlePart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	part := r.PathValue("part")
	tutDir := filepath.Join(s.tutorialsDir, slug)
	tut, err := store.ReadMetadata(tutDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderPart(w, tut, tutDir, part)
}

func (s *Server) renderPart(w http.ResponseWriter, tut *store.Tutorial, tutDir, part string) {
	src, err := os.ReadFile(filepath.Join(tutDir, part))
	if err != nil {
		http.Error(w, "part not found", http.StatusNotFound)
		return
	}
	content, err := RenderMarkdown(src)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.layoutTmpl.Execute(w, map[string]any{
		"Title":       tut.Title,
		"Tutorial":    tut,
		"CurrentPart": part,
		"Content":     template.HTML(content),
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/serve/... -v
```

Expected: all four serve tests PASS (TestListPage, TestTutorialPage, TestSeriesPartPage, TestNotFound).

- [ ] **Step 5: Commit**

```bash
git add internal/serve/server.go internal/serve/server_test.go
git commit -m "feat: add web server with list, tutorial, and part routes"
```

---

### Task 10: `lathe serve` and `lathe open` Commands

**Files:**
- Create: `cmd/serve.go`
- Create: `cmd/open.go`

- [ ] **Step 1: Write `cmd/serve.go`**

```go
// cmd/serve.go
package cmd

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/serve"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the tutorial web server and open the browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := config.TutorialsDir()
		if err != nil {
			return err
		}
		srv := serve.NewServer(dir)
		url := fmt.Sprintf("http://localhost:%d", servePort)
		fmt.Printf("Serving tutorials at %s\n", url)
		openBrowser(url)
		return http.ListenAndServe(fmt.Sprintf(":%d", servePort), srv.Handler())
	},
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		return
	}
	exec.Command(cmd, url).Start()
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 4242, "port to listen on")
	rootCmd.AddCommand(serveCmd)
}
```

- [ ] **Step 2: Write `cmd/open.go`**

```go
// cmd/open.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open <slug>",
	Short: "Open a tutorial in the browser (requires lathe serve to be running)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		url := fmt.Sprintf("http://localhost:%d/%s/", servePort, slug)
		fmt.Printf("Opening %s\n", url)
		openBrowser(url)
		return nil
	},
}

func init() {
	openCmd.Flags().IntVar(&servePort, "port", 4242, "port where lathe serve is running")
	rootCmd.AddCommand(openCmd)
}
```

- [ ] **Step 3: Build and smoke test**

```bash
go build -o /tmp/lathe .
/tmp/lathe serve &
sleep 1
curl -s http://localhost:4242/ | grep -o '<title>.*</title>'
kill %1
```

Expected: `<title>Lathe — Tutorials</title>`

- [ ] **Step 4: Commit**

```bash
git add cmd/serve.go cmd/open.go
git commit -m "feat: add lathe serve and lathe open commands"
```

---

### Task 11: Verify Subprocess

**Files:**
- Create: `internal/verify/skills/lathe-verify.md`
- Modify: `internal/verify/verify.go` (replace stub with real implementation)
- Create: `internal/verify/verify_test.go`

- [ ] **Step 1: Write the verify skill file**

```markdown
<!-- internal/verify/skills/lathe-verify.md -->
# Lathe Tutorial Verifier

Verify that a technical tutorial works end-to-end on this machine by working through it step by step.

## Setup

The tutorial to verify is at the absolute path in the `LATHE_TUTORIAL_DIR` environment variable.
Your working directory (the project dir) is a temp directory — write all code and create all files here only.
Never write files outside the current working directory.

## Process

1. Read `$LATHE_TUTORIAL_DIR/metadata.json` to determine the files to check:
   - If `series: true`, process each filename listed in `parts` in order
   - If `series: false`, process `index.md`
2. For each file, read it completely, then work through every step in order:
   - Create any code files the tutorial instructs you to create (in the current working directory)
   - Run every command shown in the tutorial
   - At each "Checkpoint" section, run the exact verification command shown
3. Track the step number (1-indexed, reset per part)
4. If any command fails or produces unexpected output, record the failure and stop immediately

## Reporting: Success

Write `$LATHE_TUTORIAL_DIR/verify-result.json`:
```json
{"status": "verified", "checked_at": "<RFC3339 timestamp>"}
```

Then update `$LATHE_TUTORIAL_DIR/metadata.json`: change the `"status"` field value to `"verified"`. Do not modify any other fields.

## Reporting: Failure

Write `$LATHE_TUTORIAL_DIR/verify-result.json`:
```json
{
  "status": "failed",
  "part": "<filename of the part that failed, e.g. part-02.md>",
  "failed_step": <step number as integer>,
  "error": "<exact error message or output from the failed command>"
}
```

Then update `$LATHE_TUTORIAL_DIR/metadata.json`: change the `"status"` field value to `"failed"`. Do not modify any other fields.

## Rules

- Only create or modify files inside the current working directory
- Never modify the tutorial markdown files
- If a required tool is not installed (e.g., `zig` binary not found), treat it as a failure: `"error": "required tool not installed: zig"`
- Count steps per part, resetting to 1 for each new part file
```

- [ ] **Step 2: Write the failing test**

```go
// internal/verify/verify_test.go
package verify_test

import (
	"os"
	"testing"

	"github.com/devenjarvis/lathe/internal/verify"
)

func TestSpawnVerifierMissingClaude(t *testing.T) {
	tutDir := t.TempDir()

	// Remove PATH so claude binary can't be found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	err := verify.SpawnVerifier("test-slug", tutDir)
	if err == nil {
		t.Error("SpawnVerifier() expected error when claude not in PATH, got nil")
	}
}
```

- [ ] **Step 3: Run test to verify it fails (or panics with stub)**

```bash
go test ./internal/verify/... -run TestSpawnVerifierMissingClaude -v
```

Expected: test FAILS — the stub returns nil and the test expects an error.

- [ ] **Step 4: Write the real implementation**

```go
// internal/verify/verify.go
package verify

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed skills/lathe-verify.md
var verifySkillContent string

func SpawnVerifier(slug, tutorialDir string) error {
	tempDir, err := os.MkdirTemp("", "lathe-verify-"+slug+"-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Write the embedded skill into the temp dir so claude can discover it
	skillDir := filepath.Join(tempDir, ".claude", "skills", "lathe-verify")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("create skill dir: %w", err)
	}
	skillPath := filepath.Join(skillDir, "lathe-verify.md")
	if err := os.WriteFile(skillPath, []byte(verifySkillContent), 0644); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("write skill: %w", err)
	}

	prompt := fmt.Sprintf(
		"Use the /lathe-verify skill to verify the tutorial. "+
			"LATHE_TUTORIAL_DIR is set to %q.",
		tutorialDir,
	)

	cmd := exec.Command("claude",
		"--project-dir", tempDir,
		"--dangerously-skip-permissions",
		"-p", prompt,
	)
	cmd.Env = append(os.Environ(), "LATHE_TUTORIAL_DIR="+tutorialDir)

	if err := cmd.Start(); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("start verifier: %w", err)
	}

	// Detach: clean up temp dir when subprocess exits, don't block
	go func() {
		cmd.Wait()
		os.RemoveAll(tempDir)
	}()

	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/verify/... -run TestSpawnVerifierMissingClaude -v
```

Expected: `PASS — TestSpawnVerifierMissingClaude`

- [ ] **Step 6: Verify full build still works**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/verify/verify.go internal/verify/verify_test.go internal/verify/skills/lathe-verify.md
git commit -m "feat: implement verify subprocess with embedded skill"
```

---

### Task 12: `/lathe` Generation Skill

**Files:**
- Create: `.claude/skills/lathe/lathe.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p .claude/skills/lathe
```

- [ ] **Step 2: Write `.claude/skills/lathe/lathe.md`**

```markdown
# Lathe — Tutorial Generator

Generate hands-on technical tutorials for any topic on demand.

## When Invoked

The user invokes you by saying something like `/lathe "build a digital synth in Zig"` or `/lathe how to build a compiler in Rust`. Extract the topic from their message.

1. Ask: **"What's your experience level going in — beginner, some familiarity, or experienced in adjacent areas?"**
2. If the topic could mean meaningfully different things (e.g., "build a web server" — what language? embedded? full-stack?), ask one clarifying question. Otherwise proceed.
3. Generate the tutorial(s).

## Single vs Series

Generate a **series** when ALL of these are true:
- The topic produces something non-trivial at the end (a working database, a compiler, a synth, a game engine)
- There are 3 or more natural milestones, each producing something runnable and testable independently
- Covering it well would exceed ~2500 words for a single post

Generate a **single tutorial** when the topic is focused and completable in one sitting.

## Tutorial Format

Every tutorial or series part must follow this structure:

```
# [Title]

## What You'll Build

One to two paragraphs: the concrete end state, why it's interesting, what you'll understand by the end.

## Prerequisites

Bullet list of what the reader needs installed and roughly knows going in.

## [Step 1: Clear, active title]

Explain *why* this step exists and what problem it solves — not just what to type.
Then show the code or command. Write it so the reader understands it, not just copies it.

## [Step 2: ...]

...

## Checkpoint

**Run this to verify your work so far:**
```bash
<the exact command>
```
Expected output:
```
<what they should see>
```
```

For **series**, each part must:
- Open with "By the end of this part, you'll have [specific, concrete thing]"
- Close with a Checkpoint section
- Leave the reader with something working they can run

## Writing Quality Standards

- Lead with the *why*, follow with the *what*
- Treat the reader as intelligent but unfamiliar with this specific domain
- Show the mental model, not just the mechanics
- When there's a non-obvious design choice, explain the trade-off
- Code blocks should be complete enough to run (no unexplained `...` gaps)

## Output Files

Write to `/tmp/lathe-<slug>/` where slug is the topic in kebab-case:
- "build a digital synth in Zig" → `/tmp/lathe-digital-synth-zig/`
- Series: `part-01.md`, `part-02.md`, `part-03.md`, … (zero-padded, sorted alphabetically)
- Single: `index.md`

Determine the slug before writing any files.

## After Writing

Run:
```bash
lathe store --verify /tmp/lathe-<slug>
```

Then tell the user:
- "**Tutorial saved.** Run `lathe serve` to open it at http://localhost:4242"
- For a series: "This is a [N]-part series. Part 1 gets you to [X], Part 2 to [Y], …"
- "Verification is running in the background — the ⏳ badge will turn ✅ when done."

## Stay in Session

Do not end the session. Remain available for:
- Follow-up questions ("why did we structure it this way?")
- Customization requests ("make Part 2 more advanced")
- Post-work review ("how'd I do on the checkpoint?")
- Edge case exploration ("what happens if the buffer overflows?")

You are their expert guide for this topic. Stay engaged.
```

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/lathe/lathe.md
git commit -m "feat: add /lathe generation skill"
```

---

### Task 13: Install and End-to-End Smoke Test

**Files:** none (install + manual verification)

- [ ] **Step 1: Install the binary**

```bash
go install .
```

Expected: `lathe` binary installed to `$(go env GOPATH)/bin/lathe`. Verify: `which lathe` returns a path.

- [ ] **Step 2: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS. Note the count — should be at least 9 tests across config, store, and serve packages.

- [ ] **Step 3: Smoke test the full CLI**

```bash
# Create a sample tutorial manually to test the server
mkdir -p /tmp/lathe-smoke-test
cat > /tmp/lathe-smoke-test/index.md << 'EOF'
# Smoke Test Tutorial

## What You'll Build

A simple "hello, world" in Go to verify lathe is working.

## Prerequisites

- Go installed

## Step 1: Write the program

Create `hello.go`:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, lathe!")
}
```

## Checkpoint

**Run this to verify your work:**
```bash
go run hello.go
```
Expected output:
```
Hello, lathe!
```
EOF

lathe store /tmp/lathe-smoke-test
lathe list
```

Expected: `lathe list` shows `smoke-test` with `✅ verified` status.

- [ ] **Step 4: Smoke test the web server**

```bash
lathe serve &
sleep 1
curl -s http://localhost:4242/ | grep -o 'Smoke Test Tutorial'
curl -s http://localhost:4242/lathe-smoke-test/ | grep -o 'Hello, lathe'
kill %1
```

Expected: both curl commands return matching text.

- [ ] **Step 5: Add `.gitignore`**

```
# .gitignore
.superpowers/
/lathe
```

- [ ] **Step 6: Final commit**

```bash
git add .gitignore
git commit -m "chore: add .gitignore"
```

---

## Self-Review

**Spec coverage check:**
- ✅ `lathe store <path> [--verify]` — Task 5
- ✅ `lathe serve [--port N]` — Task 10
- ✅ `lathe list` — Task 6
- ✅ `lathe open <slug>` — Task 10
- ✅ Metadata model with status values — Task 3
- ✅ Single vs series detection — Task 4
- ✅ Markdown rendering with syntax highlighting — Task 7
- ✅ HTML templates with sidebar + status badge — Tasks 8 & 9
- ✅ Background verification subprocess — Task 11
- ✅ Verify skill embedded in binary — Task 11
- ✅ `/lathe` generation skill — Task 12
- ✅ Verify skill content (lathe-verify.md) — Task 11
- ✅ Tutorials stored in `~/.lathe/tutorials/` — Task 4
- ✅ Status updates on page refresh (metadata read per request) — Task 9

**No placeholders found.** All steps contain complete code or exact commands.

**Type consistency verified:** `store.Status`, `store.Tutorial`, `store.StatusVerified/Verifying/Failed`, `serve.NewServer`, `verify.SpawnVerifier` — all consistent across tasks.
