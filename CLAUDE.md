# CLAUDE.md

Orientation for Claude Code working in this repo.

## What this is

Lathe is a Go CLI plus a pair of Claude Code skills that together generate, store, serve, and verify hands-on technical tutorials. See `README.md` for user-facing docs and `docs/superpowers/specs/2026-05-03-lathe-design.md` for the design spec.

The boundary is strict: **skills generate content; the CLI owns durable state.** Don't move generation logic into Go and don't have skills write to `~/.lathe/tutorials/` directly — they call `lathe store` instead.

## Layout

```
main.go                           cobra entrypoint
cmd/
  root.go                         rootCmd ("lathe")
  list.go, open.go, rm.go, serve.go, store.go, verify.go    one subcommand per file
internal/
  config/                         TutorialsDir() → ~/.lathe/tutorials
  store/
    metadata.go                   Tutorial struct, Status enum, Read/WriteMetadata
    store.go                      Store(), Delete(), copyDir/copyFile, detectParts, SlugToTitle
  serve/
    server.go                     net/http handlers (list, tutorial, part, delete)
    renderer.go                   goldmark + chroma markdown rendering
    layout.html, list.html        embed.FS page templates
    components.html               shared {{define}} partials (head, badge, themeToggle)
    styles.css                    the design system (tokens + components), embedded & injected inline
    static/fonts/*.woff2          embedded latin-subset fonts (Fraunces, Newsreader, JetBrains Mono)
  verify/
    verify.go                     StartVerification + SpawnVerifier — detached `claude` subprocess
    skills/lathe-verify.md        embedded skill (go:embed) shipped to subprocess temp dir
.claude/skills/lathe/lathe.md     /lathe generation skill (user-invoked)
docs/design-system.md            design-system docs (tokens, type scale, components, how-to-add)
docs/superpowers/                 specs/ and plans/
```

## Build, test, run

```bash
go build -o lathe                 # build the binary
go test ./...                     # run all tests
go vet ./...                      # static checks
```

There is no top-level test runner script — tests are plain `go test`. The `/lathe` (`lathe`) binary built from this repo is gitignored at the repo root.

## Architecture notes

- **`cmd/serve.go`** registers `--port` on its command's flags but stores it in the package-level `servePort` variable, which `cmd/open.go` also reads. Keep them in sync if you add new commands that need the port.
- **`internal/serve/server.go`** uses Go 1.22+ method-and-pattern routing (`mux.HandleFunc("GET /{slug}/", …)`). `safeTutorialPath` defends against path traversal by checking the joined path stays under `tutorialsDir` — preserve that check on any new route.
- **`internal/verify/verify.go`** `StartVerification` writes `metadata.json` with status=`verifying` *before* spawning the verifier so the UI can show the in-flight badge even if subprocess spawn fails. The `TestStartVerificationSetsVerifying` test depends on this ordering. It conflict-guards against an already `verifying`/`extending` tutorial. All three triggers (`lathe verify`, `lathe store --verify`, the web button) funnel through it.
- **`internal/verify/verify.go`** embeds `skills/lathe-verify.md` via `//go:embed` and writes it into a fresh temp dir per invocation, then runs `claude --add-dir <temp> --add-dir <tutorialDir> --dangerously-skip-permissions -p <prompt>` with `cmd.Dir` pinned to the temp dir (so files land there, not in the user's repo). Runs under a `context.WithTimeout(verifyTimeout)` (20 min); the detached goroutine `cmd.Wait()`s, calls `finalizeVerify` to flip a still-`verifying` status to `failed` (timeout/crash fallback), captures output to `verify.log`, and cleans up the temp dir.
- **HTML templates** are `embed.FS`-bundled (`internal/serve/*.html`) so the binary is self-contained. They use a small `add` funcMap for 1-indexed part numbering. `components.html` is parsed into **both** the layout and list template sets (with `funcMap` attached to both) so its shared partials are available everywhere — see `NewServer`.
- **Design system**: `styles.css` is the single source of truth for all UI styling — light/dark color tokens, `@font-face`, base typography, and every component class. It's `go:embed`'d as `stylesCSS`, exposed to templates as `.CSS`, and injected inline via the `{{define "head"}}` partial (alongside `.HighlightCSS`) so there's no extra request and no FOUC. **Status and callout colors are CSS tokens in `styles.css`, not inline in the templates.** Full docs in `docs/design-system.md`.
- **Fonts** are latin-subset `woff2` (`internal/serve/static/fonts/`), `go:embed`'d and served at flat `/_static/<name>.woff2` (single-segment route + explicit whitelist preserved; `handleStatic` resolves `.woff2` names into the `fonts/` subdir). The UI stays 100% offline.
- **Markdown rendering** uses goldmark with the `tango` (light) / `gruvbox` (dark) Chroma styles, chosen to harmonize with the warm palette; the code-block container background is owned by our `--code-bg` token via `pre.chroma` in `styles.css`, so only syntax-token hues come from Chroma. Tests assert that `<pre>` and a highlight class appear in output (and spot-check `#8f5902`/`#fe8019`), so don't disable highlighting or swap styles without updating `renderer_test.go`.

## Conventions

- One cobra subcommand per file in `cmd/`, registered via `init()` calling `rootCmd.AddCommand(...)`.
- Errors flow up through `RunE`; the root `Execute()` exits non-zero on any error.
- Keep `internal/` packages free of cobra imports — they should be usable from tests directly.
- Skills are markdown files. The `/lathe` skill is checked into `.claude/skills/`; the `/lathe-verify` skill is *embedded* into the binary because it ships with the runtime, not the repo.
- Status values are an enum (`store.Status`): `unverified` (default after store; renders no badge), `verifying`, `verified`, `failed`, `skipped` (required tool not installed — not a failure), `extending`. New states should be added there and reflected in `cmd/list.go` `statusBadge`, the `{{define "badge"}}` partial in `components.html`, and the `--badge-*` tokens + `.badge.<status>` rule in `styles.css` (see "how to add a new status" in `docs/design-system.md`).

## Things to avoid

- Verification is **opt-in / on-demand**: it runs only when the user asks (the `lathe verify <slug>` command, the `--verify` flag on `lathe store`, or the "Verify this tutorial" web button — all routed through `verify.StartVerification`). Storing never auto-verifies; the default status is `unverified`. Don't add a `lathe status` command — status is surfaced via `lathe list` and the web UI.
- Don't add tutorial editing or sharing commands without checking with the user — the v1 scope is deliberately narrow. (Deletion is supported via `lathe rm <slug>` and the `×` button on the web list page; both go through `store.Delete` / `safeTutorialPath`.)
- Don't have the verify skill modify the tutorial source markdown — it's read-only with respect to the tutorial directory, and only writes `verify-result.json` and the `status` field of `metadata.json`.
- Don't add OS-level sandboxing (sandbox-exec, Docker) for verification unless explicitly asked — soft isolation via `cmd.Dir`-into-a-temp-dir plus scoped `--add-dir` grants is the chosen tradeoff.

## Commit style

Conventional commits (`feat:`, `fix:`, `chore:`, `refactor:`) — match the existing log. Keep subject lines short and imperative. Tests typically land in the same commit as the code they cover.
