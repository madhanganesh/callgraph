# callgraph

CLI tool + Vim plugin for navigating and understanding code:

- **Callers** (incoming) and **callees** (outgoing) call graphs via LSP `callHierarchy/{incoming,outgoing}Calls`.
- **Interface picker** via LSP `textDocument/implementation` — when the cursor is on an interface method, prompt for an impl (auto-select if only one survives the mock/test filter).
- **Callee classification** with API 🌐 / DB 🛢️ / thread 🧵 icons + call-site detail (URL host, SQL table). Rules per language.
- **Summarize** subcommand pipes the function under cursor to an LLM CLI (default `claude -p`) and prints the response. Cursor-rooted via LSP, two-layer cache.
- **Vim-only conveniences**: `<leader>cp` free-form prompt with context-prefix expansion, `<leader>c{f,l,m}` clipboard yank helpers.

Supports Go, Python, Rust via the `Language` interface.

## Architecture

```
main.go ─┬─ summarize subcommand → internal/summarize.Run → LSP + LLM CLI
         └─ default              → lang.Detect → lsp.NewClient
                                  → graph.Build{Caller,Callee}Tree
                                  → output.Format{Tree,CalleeTree,JSON}
```

### Key packages

- `internal/lang/` — Language interface + Go/Python/Rust impls.
  - `FindRoot`, `RelPkg`, `EnclosingFunc`, `LSPCommand`, `LanguageID`
  - `ClassifyRules() []classify.Rule` and `ThreadSpawnPattern() *regexp.Regexp` for callee classification (return nil to skip).
- `internal/lsp/` — Language-agnostic JSON-RPC client over stdin/stdout. Methods: `DidOpen`, `PrepareCallHierarchy`, `IncomingCalls`, `OutgoingCalls`, `Implementation`. Handles Content-Length framing + server-initiated requests.
- `internal/graph/` — Caller and callee tree builders.
  - **Callers**: enclosing-function rooted; recursive `incomingCalls`; dedup by URI+line; transient-error retry loop (10×, 500ms).
  - **Callees**: cursor-rooted (no `EnclosingFunc` snap); recursive `outgoingCalls`; sorts by source-order via `FromRanges`; per-traversal source cache (`srcCache`); drops plain external callees as noise; classifies API/DB/thread leaves; `extractEndpoint` (URL) and `extractTable` (SQL) for `Detail`.
  - **Interface picker**: `resolveImplementations` + `filterRealImpls` (drops `_test.go`, `/mock`, `/fake` paths and pkgs containing "mock"/"fake"). 0 → use item; 1 → auto-select via `prepareItemAt`; 2+ → return `Implementations` for plugin to render picker.
- `internal/classify/` — `Kind` enum, icons, `Rule` (regex → Kind), `MustRule`.
- `internal/output/` — JSON + ASCII tree formatters; callee path uses `calleeLabel` to render API URL, DB table, and pkg suffix.
- `internal/summarize/` — Self-contained subcommand:
  - Resolves symbol via `prepareCallHierarchy`. If the resolved item's body extracts cleanly, summarize it directly; otherwise treat as interface method, run `Implementation`, filter mocks/tests, then auto-select or return picker list.
  - `functionEnd`: Go via `go/parser` AST, Python via indentation, Rust via brace depth.
  - **Two-layer cache** in `os.UserCacheDir()/callgraph/summaries/` (override with `CALLGRAPH_CACHE_DIR`):
    - Position cache key = `sha256(file ‖ line ‖ col ‖ file-content ‖ llmCmd)` → full `Result` JSON. Hits skip LSP entirely.
    - Body cache key = `sha256(llmCmd ‖ prompt)` → LLM response text. Hits skip the LLM call when LSP runs but resolves to an unchanged body.
- `vim/` — Vimscript plugin.
  - `plugin/callgraph.vim`: commands + default mappings (gated on key being free).
  - `autoload/callgraph.vim`: all logic. Two parallel picker stacks — one for the call-graph view (`s:show_picker`, `s:resolve_pick`), one for summarize (`s:show_summary_picker`, `s:sum_resolve_pick`). Plain text popup (`s:show_text_popup` / `s:show_text_float`) used by summarize and prompt. Repo-relative path via `git rev-parse --show-toplevel`; enclosing name via per-filetype regex walking back.

## Vim mappings (defaults)

| Mapping | What it does |
|---|---|
| `<leader>cc` | Callers graph (enclosing function rooted) |
| `<leader>cd` | Callees graph (cursor symbol rooted) |
| `<leader>cs` | LLM summary of cursor symbol |
| `<leader>cp` | Free-form prompt → popup; "in this {file,method,line}," prefixes auto-expand to `Context: ...` |
| `<leader>cf` / `cl` / `cm` | Yank repo-relative `file` / `file:line` / `file methodname` to `+` and `*` |

## Conventions

- Line/col in public API and CLI flags: **1-based**. Converted to 0-based at the LSP boundary.
- Tree output: top-down, `|__` connectors, `funcName (package)`. Pkg trimmed if line >80 chars.
- Callee labels: API → `🌐 NAME host/path`, DB → `🛢️ name → table`, thread → `🧵 name (pkg)`.
- gopls `Detail` format is `pkg/path • file.go` — strip `" • "` to get pkg.
- For files outside the module root (stdlib/vendor), prefer the LSP `Detail` over computed `RelPkg`.

## Known issues / quirks

- gopls sometimes returns the wrong package in `Detail` of `From` items in `incomingCalls` (off by one). Fixed by parsing the `package` clause from the source file directly when needed.
- LSP servers need time to index after `didOpen`. The retry loop (10×, 500ms on transient errors) handles this; first invocation on a large project can take a few seconds.
- Python `EnclosingFunc` is a backward-walking regex — doesn't perfectly handle nested functions.
- Rust `EnclosingFunc` brace-depth counter doesn't exclude braces in strings or comments.
- For summarize: cache keys include `col`, so pressing `<leader>cs` with the cursor on different chars of the same identifier yields cache misses (still hits the body cache, so LLM is skipped — only LSP re-runs). Could be made column-tolerant if it becomes annoying.
- Module path is `github.com/madhanganesh/callgraph`. Local directory still `go-call-graph/` pending physical rename + GitHub repo rename.

## Build and test

```sh
go build -o callgraph .
go vet ./...
go test ./test/go/... ./test/python/... ./test/rust/...

# Smoke
./callgraph --file=test/go/testdata/simple/main.go --line=21 --format=tree
./callgraph summarize --file=test/go/testdata/simple/main.go --line=21 --col=6 \
    --llm-cmd='cat'   # echoes prompt instead of calling an LLM
```

## Possible next steps

- Make summarize cache column-tolerant (key by `(file, line, content-hash)` after walking to the line's start non-whitespace).
- VS Code extension shelling out to the same `summarize` and call-graph subcommands.
- Recursive summary: summarize a function plus one level of its callees in one prompt.
- Add `--lang` flag to override auto-detection.
- Extend classification rules + thread patterns with more libraries as users hit gaps.
- Package for Homebrew / release binaries.
