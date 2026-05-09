package serve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/devenjarvis/lathe/internal/store"
)

// maxQuestionBytes caps the JSON request body for /-/ask. Questions are
// expected to be short prose; anything bigger is almost certainly abuse.
const maxQuestionBytes = 8 << 10 // 8 KiB

// handleAsk streams an answer to a question about the part the user is
// currently reading. It spawns a tightly-scoped read-only `claude` subprocess
// (via exec.CommandContext bound to r.Context() so disconnect kills it) and
// re-streams the assistant text deltas to the browser as Server-Sent Events.
//
// SSE wire format:
//
//	data: <chunk>\n\n             — answer text chunks
//	event: done\ndata: {}\n\n     — clean completion
//	event: error\ndata: <msg>\n\n — subprocess or stream error
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	part := r.PathValue("part")

	// Defense in depth: only .md files are valid parts.
	if !strings.HasSuffix(part, ".md") {
		http.NotFound(w, r)
		return
	}

	tutDir, ok := s.safeTutorialPath(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	tut, err := store.ReadMetadata(tutDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	partPath, ok := s.safeTutorialPath(slug, part)
	if !ok {
		http.NotFound(w, r)
		return
	}
	articleBody, err := os.ReadFile(partPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Cap the request body. http.MaxBytesReader returns an error from Read once
	// the limit is exceeded; the json decoder surfaces that as a decode error.
	r.Body = http.MaxBytesReader(w, r.Body, maxQuestionBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request body too large", http.StatusBadRequest)
		return
	}
	if len(raw) == 0 {
		http.Error(w, "empty request body", http.StatusBadRequest)
		return
	}
	var payload struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	question := strings.TrimSpace(payload.Question)
	if question == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}

	system, user := buildAskPrompt(tut, part, string(articleBody), question)

	// Build the subprocess. Bind to the request context so client disconnect
	// kills the subprocess for free — no goroutines needed.
	cmd := exec.CommandContext(r.Context(), "claude",
		"--bare",
		"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--project-dir", tutDir,
		"--allowedTools", "Read,Glob,Grep",
		"--dangerously-skip-permissions",
		"--system-prompt", system,
		user,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "stream init failed", http.StatusInternalServerError)
		return
	}
	// Drop stderr — without this, a chatty subprocess can fill the OS pipe
	// buffer and deadlock the handler waiting on Wait().
	cmd.Stderr = io.Discard

	// SSE headers must go out before any frame is written. After the first
	// write, we cannot change status — errors after this must be SSE-framed.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		writeSSEError(w, flusher, "subprocess start failed")
		return
	}

	scanner := bufio.NewScanner(stdout)
	// Partial-message JSON events from claude can exceed the default 64KB
	// scanner buffer when they contain large code blocks; bump to 1MB.
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var obj any
		if err := json.Unmarshal(line, &obj); err != nil {
			// Non-JSON output (logs, banners) is harmless to skip.
			continue
		}
		text := extractTextDelta(obj)
		if text == "" {
			continue
		}
		writeSSEData(w, flusher, text)
	}

	// Drain stdout fully before Wait returns; reading the err afterward.
	waitErr := cmd.Wait()
	if waitErr != nil {
		writeSSEError(w, flusher, "subprocess error")
		return
	}
	fmt.Fprint(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

// writeSSEData emits a `data:` SSE frame. The SSE spec requires that newlines
// in the payload be sent as separate `data:` lines; we honor that so that
// multi-line model output renders correctly on the client.
func writeSSEData(w io.Writer, flusher http.Flusher, text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
	flusher.Flush()
}

// writeSSEError emits an `error` SSE event. The message is intentionally short
// — we don't pipe stderr here, since it could be large and may leak details.
func writeSSEError(w io.Writer, flusher http.Flusher, msg string) {
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", msg)
	flusher.Flush()
}

// extractTextDelta walks the JSON envelope of a claude --output-format
// stream-json line and returns any assistant text it carries. It handles the
// two common shapes:
//
//  1. content_block_delta with a text_delta:
//     {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
//     (optionally wrapped in {"type":"stream_event","event":{...}})
//  2. assistant message with content blocks:
//     {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
//
// Anything else (tool_use, tool_result, message_start/stop, ping, system
// banners) returns "" and is skipped by the caller.
func extractTextDelta(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}

	// Unwrap stream_event envelope.
	if t, _ := m["type"].(string); t == "stream_event" {
		if inner, ok := m["event"].(map[string]any); ok {
			return extractTextDelta(inner)
		}
	}

	// Shape 1: content_block_delta with text_delta.
	if t, _ := m["type"].(string); t == "content_block_delta" {
		if delta, ok := m["delta"].(map[string]any); ok {
			if dt, _ := delta["type"].(string); dt == "text_delta" {
				if s, ok := delta["text"].(string); ok {
					return s
				}
			}
		}
		return ""
	}

	// Shape 2: full assistant message.
	if t, _ := m["type"].(string); t == "assistant" {
		if msg, ok := m["message"].(map[string]any); ok {
			if content, ok := msg["content"].([]any); ok {
				var b strings.Builder
				for _, block := range content {
					bm, ok := block.(map[string]any)
					if !ok {
						continue
					}
					if bt, _ := bm["type"].(string); bt == "text" {
						if s, ok := bm["text"].(string); ok {
							b.WriteString(s)
						}
					}
				}
				return b.String()
			}
		}
	}

	return ""
}

// buildAskPrompt produces the (system, user) prompt pair sent to the claude
// subprocess. The system prompt embeds the full text of the part the user is
// currently reading, plus — for series — a list of sibling parts the model
// can consult via Read/Glob/Grep. The user prompt is the question verbatim.
func buildAskPrompt(tut *store.Tutorial, part, articleBody, question string) (system, user string) {
	var b strings.Builder

	title := ""
	if tut != nil {
		title = tut.Title
	}

	b.WriteString("You are a helpful assistant answering questions about a hands-on technical tutorial.\n\n")
	fmt.Fprintf(&b, "The tutorial is titled %q.\n", title)
	fmt.Fprintf(&b, "The user is currently reading the part %q.\n\n", part)
	fmt.Fprintf(&b, "The full text of %q is included below.\n\n", part)

	if tut != nil && tut.Series && len(tut.Parts) > 1 {
		// Build the list of *other* parts so the model knows what it can
		// consult via Read/Glob/Grep.
		var siblings []string
		for _, p := range tut.Parts {
			if p == part {
				continue
			}
			siblings = append(siblings, p)
		}
		if len(siblings) > 0 {
			b.WriteString("This tutorial is a series. Other parts are also available in the same directory and you have read-only Read/Glob/Grep access if you need to consult them:\n")
			for _, p := range siblings {
				fmt.Fprintf(&b, "  - %s\n", p)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("Answer the user's question concisely and accurately, citing specific parts of the tutorial when relevant. Do not write or modify any files.\n\n")
	fmt.Fprintf(&b, "--- BEGIN %s ---\n", part)
	b.WriteString(articleBody)
	if !strings.HasSuffix(articleBody, "\n") {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "--- END %s ---\n", part)

	return b.String(), question
}
