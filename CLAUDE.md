# callgraph

CLI tool + Vim plugin that shows the call graph of a function, in either
direction: upward (callers, incoming) or downward (callees, outgoing).
Supports Go, Python, and Rust via LSP. Downward mode also classifies nodes
by kind (API 🌐, DB 🛢️, thread 🧵) — currently Go-only.

## Project status

- Phase 1 (CLI): complete and working
- Phase 2 (Vim plugin): complete, works in Vim 8.2+ and Neovim 0.9+
- Phase 3 (multi-language): complete -- Go, Python, Rust via Language interface
- Tested: Go (gopls), Python (pyright + pylsp). Rust not yet tested with real project.

## Architecture

```
main.go → lang.Detect(file) → Language interface
        → lsp.NewClient(rootDir, lang.LSPCommand())
        → graph.BuildCallerTree(client, lang, file, root, line, col, depth)
        → output.FormatTree / FormatJSON
```

### Key packages

- `internal/lang/` — Language interface + implementations (golang.go, python.go, rust.go)
  - `FindRoot(file)` — walks up to find go.mod / pyproject.toml / Cargo.toml
  - `RelPkg(file, root)` — display-friendly relative package path
  - `EnclosingFunc(src, line)` — finds function enclosing cursor (Go: AST, Python/Rust: regex)
  - `LSPCommand()` — command to start LSP server
  - `LanguageID()` — LSP language identifier
- `internal/lsp/` — Language-agnostic JSON-RPC client over stdin/stdout
  - Starts LSP server as subprocess, does initialize handshake
  - Methods: DidOpen, PrepareCallHierarchy, IncomingCalls
  - Handles Content-Length framing, server-initiated requests
- `internal/graph/` — Builds caller tree recursively via LSP callHierarchy
  - Retry logic: up to 10 attempts with 500ms backoff (LSP indexing race)
  - Deduplication by URI+line to prevent infinite loops on recursive calls
- `internal/output/` — JSON and ASCII tree formatters
  - Tree: reverses caller tree to root-to-target paths, merges shared prefixes
- `vim/` — Vim/Neovim plugin (Vimscript)
  - `:CallGraph` command, `<leader>cc` mapping
  - Shells out to CLI with --format=json, renders popup/float with jump-to-caller

## Known issues / quirks

- gopls sometimes returns wrong package in the `Detail` field of `From` items in incomingCalls (off by one directory level). Fixed by parsing package declaration from the source file directly instead of using Detail.
- LSP servers need time to index after didOpen. The retry loop (10x, 500ms) handles this but means first invocation on a large project can take a few seconds.
- Python EnclosingFunc uses a simple regex walking backwards -- doesn't handle edge cases like nested functions perfectly. Works for typical code.
- Rust EnclosingFunc tracks brace depth to skip nested blocks, but the brace counter doesn't account for braces inside string literals or comments.
- Module path is `github.com/madhanganesh/callgraph`. The on-disk directory is still `go-call-graph/` pending a physical rename + GitHub repo rename.

## Conventions

- Line/col in public API and CLI flags: 1-based (user convention)
- Line/col in LSP calls: 0-based (LSP convention) -- converted at the boundary in BuildCallerTree
- Tree output: top-down, `|__` connectors, `funcName (package)` format
- Package names trimmed if line exceeds 80 chars

## Build and test

```sh
go build -o callgraph .
go vet ./...
# Integration tests (one package per language, fixtures under test/<lang>/testdata/):
go test ./test/go/... ./test/python/... ./test/rust/...
# Smoke test:
./callgraph --file=test/go/testdata/simple/main.go --line=21 --format=tree
```

## Possible next steps

- Add `--lang` flag to override auto-detection
- Test Rust with a real project
- Add outgoing calls (callHierarchy/outgoingCalls) support
- Add caching keyed on (file, func, git-sha)
- Package for Homebrew / release binaries
