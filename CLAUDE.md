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
  list.go, open.go, rm.go, serve.go, store.go    one subcommand per file
internal/
  config/                         TutorialsDir() → ~/.lathe/tutorials
  store/
    metadata.go                   Tutorial struct, Status enum, Read/WriteMetadata
    store.go                      Store(), Delete(), copyDir/copyFile, detectParts, SlugToTitle
  serve/
    server.go                     net/http handlers (list, tutorial, part, delete)
    renderer.go                   goldmark + chroma markdown rendering
    layout.html, list.html        embed.FS templates
  verify/
    verify.go                     SpawnVerifier — detached `claude` subprocess
    skills/lathe-verify.md        embedded skill (go:embed) shipped to subprocess temp dir
.claude/skills/lathe/lathe.md     /lathe generation skill (user-invoked)
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
- **`internal/store/store.go`** writes `metadata.json` *before* spawning the verifier so the UI can show the `verifying` badge even if subprocess spawn fails. The `TestStoreWithVerifyStatus` test depends on this ordering.
- **`internal/verify/verify.go`** embeds `skills/lathe-verify.md` via `//go:embed` and writes it into a fresh temp dir per invocation, then runs `claude --project-dir <temp> --dangerously-skip-permissions -p <prompt>`. The subprocess is detached; a goroutine `cmd.Wait()`s only to clean up the temp dir.
- **HTML templates** are `embed.FS`-bundled (`internal/serve/*.html`) so the binary is self-contained. They use a small `add` funcMap for 1-indexed part numbering.
- **Markdown rendering** uses goldmark with the `github` Chroma style for code highlighting; tests assert that `<pre>` and a highlight class appear in output, so don't disable highlighting without updating `renderer_test.go`.

## Conventions

- One cobra subcommand per file in `cmd/`, registered via `init()` calling `rootCmd.AddCommand(...)`.
- Errors flow up through `RunE`; the root `Execute()` exits non-zero on any error.
- Keep `internal/` packages free of cobra imports — they should be usable from tests directly.
- Skills are markdown files. The `/lathe` skill is checked into `.claude/skills/`; the `/lathe-verify` skill is *embedded* into the binary because it ships with the runtime, not the repo.
- Status values are an enum (`store.Status`): `verifying`, `verified`, `failed`. New states should be added there and reflected in `cmd/list.go` `statusBadge`, `layout.html`, and `list.html`.

## Things to avoid

- Don't add a `lathe verify` or `lathe status` command — verification is intentionally always automatic via `--verify` (see "Out of Scope" in the design spec).
- Don't add tutorial editing or sharing commands without checking with the user — the v1 scope is deliberately narrow. (Deletion is supported via `lathe rm <slug>` and the `×` button on the web list page; both go through `store.Delete` / `safeTutorialPath`.)
- Don't have the verify skill modify the tutorial source markdown — it's read-only with respect to the tutorial directory, and only writes `verify-result.json` and the `status` field of `metadata.json`.
- Don't add OS-level sandboxing (sandbox-exec, Docker) for verification unless explicitly asked — soft isolation via `--project-dir` is the chosen tradeoff.

## Commit style

Conventional commits (`feat:`, `fix:`, `chore:`, `refactor:`) — match the existing log. Keep subject lines short and imperative. Tests typically land in the same commit as the code they cover.
