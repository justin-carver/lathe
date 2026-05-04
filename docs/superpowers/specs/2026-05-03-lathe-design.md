# Lathe — Design Spec

**Date:** 2026-05-03

## Overview

Lathe is a tool for generating on-demand technical tutorials that guide engineers through any topic hands-on. It strikes a balance between "LLMs do all my thinking" and "I figure everything out alone" — the user does the work, but with expert guidance, context for the *why*, and a Claude session that stays live for follow-up questions.

Target use cases: topics where approachable documentation is scarce or thin — "build a digital synth in Zig", "build a 3D slicer from scratch", "build a database from scratch in Go".

## Architecture

Two layers with a clean boundary:

- **Skills layer** — Claude Code skills. Responsible for all generative content. Skills call CLI commands; they never own durable state.
- **CLI layer** (`lathe`, Go binary) — responsible for all deterministic work: storage, web serving, verification orchestration, state persistence.

```
User: /lathe "build a digital synth in Zig"
        │
        ▼
  [lathe skill]        generates markdown, decides single vs series
        │
        ▼  lathe store --verify <path>
  [lathe CLI]          saves files, kicks off background verification
        │
        ├── ~/.lathe/tutorials/digital-synth-zig/
        │        metadata.json   (status: "verifying")
        │        part-01.md
        │        part-02.md
        │
        └── [bg subprocess: claude --project-dir <temp-dir> + lathe-verify skill]
                 works through tutorial in temp dir
                 compiles/runs at each checkpoint
                 writes verify-result.json
                 updates metadata.json → "verified" or "failed"

User: lathe serve  →  http://localhost:PORT  (browser opens automatically)
```

## Skills

### `/lathe` — generation skill

The sole user-facing entry point.

- User invokes: `/lathe "build a digital synth in Zig"`
- Asks 1–2 scoping questions (experience level, constraints) before generating
- Decides single tutorial vs series based on topic scope:
  - **Single**: one blog-post-sized markdown file (`index.md`)
  - **Series**: multiple files (`part-01.md`, `part-02.md`, …), each building toward the final outcome with a testable stepping stone at the end
- Tutorial format: narrative explaining the *why*, code the user types themselves, a runnable/testable checkpoint at the end of each part
- Writes generated markdown to a temp output directory (`/tmp/lathe-<slug>/`), then calls `lathe store --verify /tmp/lathe-<slug>` and reports the URL to open
- Session stays live — user can ask follow-up questions, request variations, or explore edge cases naturally

### `/lathe-verify` — verification skill

Invoked only by the CLI as a background subprocess. Never called directly by the user.

- Receives the tutorial path via the project dir (`--project-dir <temp-dir>`)
- Works through the tutorial step by step: creates files, runs commands, compiles at each checkpoint
- Writes `verify-result.json` to the tutorial directory: pass/fail, failed step number, error message
- Updates `metadata.json` status to `"verified"` or `"failed"`
- Never modifies tutorial content — read-only with respect to the source markdown
- Stays within the temp working directory (enforced by `--project-dir` and skill instructions)

## CLI

### Storage

Tutorials live in `~/.lathe/tutorials/` globally — accessible from any working directory.

```
~/.lathe/tutorials/
  digital-synth-zig/
    metadata.json
    part-01.md
    part-02.md
    part-03.md
  database-from-scratch-go/
    metadata.json
    index.md
```

**`metadata.json` schema:**
```json
{
  "slug": "digital-synth-zig",
  "title": "Build a Digital Synth in Zig",
  "topic": "build a digital synth in Zig",
  "created": "2026-05-03T19:00:00Z",
  "status": "verifying",
  "series": true,
  "parts": ["part-01.md", "part-02.md", "part-03.md"]
}
```

Status values: `"verifying"` | `"verified"` | `"failed"`

On failure, a `verify-result.json` is also present:
```json
{
  "status": "failed",
  "failed_step": 3,
  "error": "zig build: error: unable to find 'std.audio'",
  "part": "part-02.md"
}
```

### Commands

```
lathe store <path> [--verify]   Copy tutorial dir to ~/.lathe/tutorials/, optionally spawn verification
lathe serve [--port N]          Start web server, open browser (default port: 4242)
lathe list                      Print all tutorials with status badges
lathe open <slug>               Open a specific tutorial in the browser
```

### Web Renderer

`lathe serve` starts a `net/http` server. Markdown rendered to HTML using [goldmark](https://github.com/yuin/goldmark) with syntax highlighting via [chroma](https://github.com/alecthomas/chroma). Browser opened automatically via `open`/`xdg-open`.

Features:
- Sidebar navigation for series (part list, current part highlighted)
- Status badge on every page: `[verifying…]` → `✅ Verified` → `❌ Failed (part-02, step 3)`
- Status read from `metadata.json` on each page load — refreshing shows updated state

## Verification Flow (end to end)

1. Skill calls `lathe store --verify ./generated/digital-synth-zig`
2. CLI copies files to `~/.lathe/tutorials/digital-synth-zig/`, writes `metadata.json` with `status: "verifying"`
3. CLI creates a temp working directory, spawns a detached subprocess running `claude` with `--project-dir <temp-dir>` and `--dangerously-skip-permissions`. The subprocess is given the verify skill (either via `--skill` flag or inline prompt — exact invocation flags to be confirmed during implementation).
4. CLI exits — skill reports tutorial saved and verification started
5. Subprocess runs `/lathe-verify` skill, working through each tutorial step in the temp dir
6. On completion, subprocess writes `verify-result.json` and updates `metadata.json`
7. Temp dir is cleaned up by the subprocess on exit
8. `lathe serve` reflects updated status on next page load

**Sandbox approach:** `--project-dir <temp-dir>` sets the subprocess's project root to the temp directory. The verify skill is written to use only relative paths and never reference paths outside the project dir. This is a soft constraint enforced by skill design rather than OS-level isolation. Hard sandboxing (macOS `sandbox-exec` or Docker) can be added later if needed.

**On verification failure:** the failed tutorial remains readable in `lathe serve` with a clear failure badge and error details. The user re-opens their Claude session and `/lathe` can use the failure context to patch or regenerate. Content changes always flow through the generation skill.

## Dependencies

- `claude` CLI in `$PATH` (reasonable assumption — this tool is Claude Code-native)
- Go standard library (`net/http`, `os/exec`)
- [goldmark](https://github.com/yuin/goldmark) — markdown rendering
- [chroma](https://github.com/alecthomas/chroma) — syntax highlighting
- [cobra](https://github.com/spf13/cobra) — CLI command structure

## Out of Scope (v1)

- Hard OS-level sandboxing for verification
- `lathe verify` / `lathe status` CLI commands (verification is always automatic via `--verify`)
- `lathe read` (glow fallback)
- Tutorial deletion or management commands
- Sharing or exporting tutorials
