# callgraph

A tool that shows the upward call graph (callers) of a Go function from your current cursor position. Designed as a standalone CLI with thin editor plugins for Vim and VS Code.

## Problem

When navigating a Go codebase, you often land inside a function and need to answer: "How did execution get here?" Today this requires manually jumping through references. The goal is a single keystroke that shows the full caller chain up to the entry point.

## Requirements

- Given a file, line, and column, produce the upward call graph (incoming calls) for the function at that position.
- Traverse callers recursively up to a configurable depth.
- Output structured data (JSON) for editor consumption and a human-readable ASCII tree for CLI use.
- Work incrementally — fast enough for interactive use on large codebases.
- Editor-agnostic core: one Go binary, thin plugins on top.

## Architecture

```
┌─────────────┐     ┌──────────────────────┐     ┌───────┐
│  Vim plugin  │────>│  callgraph (CLI) │────>│ gopls │
│              │     │  (Go binary)         │     │ (LSP) │
└─────────────┘     └──────────────────────┘     └───────┘
                              ^
┌─────────────┐               │
│  VS Code    │───────────────┘
│  extension  │
└─────────────┘
```

The CLI is the single source of logic. Editor plugins are thin wrappers that:
1. Capture cursor position.
2. Shell out to the CLI.
3. Parse the JSON response and render it.

## Chosen Approach: gopls via LSP

### Why gopls

| Approach | Pros | Cons |
|---|---|---|
| **gopls (LSP `callHierarchy`)** | Actively maintained, incremental, already handles modules, same protocol for all editors | Some LSP ceremony |
| `guru callers` | Purpose-built | Deprecated, slow on large repos |
| `go/callgraph` (library) | Most accurate (pointer analysis) | Slow startup, high memory, must analyze whole program |
| `go/ast` + `go/types` | Fast, simple | Misses interface dispatch, indirect calls |

gopls supports `callHierarchy/incomingCalls` (LSP 3.16+), which is exactly the primitive we need. It is actively maintained by the Go team, already runs in most editor setups, and handles incremental analysis so repeat queries are fast.

### How it works

1. CLI starts or connects to a `gopls` instance.
2. Sends `textDocument/prepareCallHierarchy` for the given position.
3. Recursively sends `callHierarchy/incomingCalls` for each item, up to `--depth` levels.
4. Builds a tree and outputs it.

## CLI Design

### Usage

```
callgraph --file=<path> --line=<n> --col=<n> [--depth=5] [--format=json|tree]
```

### JSON Output

```json
{
  "name": "handleRequest",
  "file": "server.go",
  "line": 115,
  "callers": [
    {
      "name": "ServeHTTP",
      "file": "server.go",
      "line": 42,
      "callers": [
        {
          "name": "main",
          "file": "main.go",
          "line": 10,
          "callers": []
        }
      ]
    }
  ]
}
```

### Tree Output (--format=tree)

```
handleRequest            (server.go:115)
  <- ServeHTTP           (server.go:42)
    <- main              (main.go:10)
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--file` | required | Absolute path to the Go source file |
| `--line` | required | Line number (1-based) |
| `--col` | 1 | Column number (1-based) |
| `--depth` | 5 | Max traversal depth |
| `--format` | `tree` | Output format: `json` or `tree` |

## Vim Plugin

### Trigger

```vim
nnoremap <leader>cc :call CallGraph()<CR>
```

### Behavior

1. Read cursor position: `expand('%:p')`, `line('.')`, `col('.')`.
2. Shell out: `system('callgraph --file=... --line=... --col=... --format=json')`.
3. Parse JSON, format as indented lines.
4. Display in `popup_atcursor()` (Vim 8.2+) or `nvim_open_win()` (Neovim).
5. Keybindings inside popup:
   - `<CR>` — jump to the selected caller.
   - `q` — close the popup.
   - `j/k` — navigate entries.

### Target file

Single file: `plugin/callgraph.vim` (or `.lua` for Neovim).

## VS Code Extension

### Approach

A TypeScript extension that:
1. Registers a command `callgraph.showCallers`.
2. Reads active editor position.
3. Shells out to the same `callgraph` CLI binary.
4. Renders the result in a TreeView panel in the sidebar.

### Alternative

VS Code's Go extension already has `gopls` running. The extension could call `vscode.commands.executeCommand('vscode.prepareCallHierarchy', ...)` directly and skip the CLI. This avoids a second `gopls` instance but ties the traversal logic to TypeScript. Decision: start with the CLI wrapper for consistency, optimize later if needed.

## Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Standalone gopls or piggyback on editor's? | Standalone | Simpler, avoids IPC with editor's LSP client. Double memory cost is acceptable for a dev tool. |
| Eager full graph vs lazy expand? | Eager up to `--depth` | Simpler for Vim popup. Lazy expand can be added later for VS Code TreeView. |
| Max depth default | 5 | Unbounded traversal can explode. 5 levels covers most practical cases. |
| Caching | Cache keyed on `(file, func, git-sha)` | Call graphs don't change often. Avoids redundant gopls round-trips. |

## Implementation Phases

### Phase 1 — CLI tool
- [ ] Scaffold Go module with cobra or plain flag parsing.
- [ ] Implement LSP client that connects to gopls.
- [ ] Implement `prepareCallHierarchy` + recursive `incomingCalls`.
- [ ] JSON and tree output formatters.
- [ ] Tests with a sample Go project.

### Phase 2 — Vim plugin
- [ ] Vimscript/Lua plugin that calls the CLI.
- [ ] Popup rendering with navigation.
- [ ] Jump-to-caller on `<CR>`.
- [ ] Package for vim-plug / lazy.nvim.

### Phase 3 — VS Code extension
- [ ] TypeScript extension scaffolding.
- [ ] TreeView rendering of call graph.
- [ ] Click-to-navigate to caller.
- [ ] Publish to VS Code marketplace.

## Extensions beyond the original design

The base design (upward call graph for Go) shipped as Phase 1. Subsequent work added the following, all built on the same CLI-as-engine + thin-plugin pattern.

### Multi-language

Go-only LSP plumbing was abstracted behind a `Language` interface (`internal/lang/`). Go uses `go/ast`; Python and Rust use regex-based heuristics for `EnclosingFunc`. Detection by file extension; project root from `go.mod` / `pyproject.toml` / `Cargo.toml`.

### Downward call graph (callees)

Mirrors the caller side using `callHierarchy/outgoingCalls`. Two design departures from the caller flow:

- **Cursor-rooted, not enclosing-function rooted.** Reading `dw.PlaceOrder(...)` and asking "what does it do?" should resolve `PlaceOrder`, not the function the user happens to be inside.
- **Classification of leaves.** Each callee is checked against per-language rule sets (`ClassifyRules`) and a thread-spawn regex matched on the call-site line. Classified leaves get an icon (🌐 API, 🛢️ DB, 🧵 thread) and an extracted `Detail` (URL host, SQL table). Unclassified external (stdlib/vendor) callees are dropped as noise; descent stops at any classified or external boundary.

### Interface picker

When the cursor symbol resolves to an interface method, the engine runs `textDocument/implementation`, filters out test fixtures and mocks (`_test.go`, `/mock`, `/fake`, packages containing "mock"/"fake"), then:

- 0 real impls → use the item itself.
- 1 → auto-resolve and continue.
- 2+ → return `Implementations` and let the plugin render a picker. The picker re-invokes the CLI rooted at the chosen impl.

The same picker mechanism is reused by the summarize subcommand.

### Summarize subcommand

```
callgraph summarize --file=... --line=... --col=... [--llm-cmd='claude -p']
```

Resolves the symbol under cursor via LSP, extracts the function body (Go via `go/parser`, Python via indent, Rust via brace depth), and pipes it to a configurable LLM CLI. JSON output supports the same `implementations` shape so the plugin can re-use the picker for interface methods.

**Two-layer cache** in `os.UserCacheDir()/callgraph/summaries/`:

| Layer | Key | Skips |
|---|---|---|
| Position | `sha256(file ‖ line ‖ col ‖ file-content ‖ llmCmd)` | LSP + LLM |
| Body     | `sha256(llmCmd ‖ prompt-with-function-body)`         | LLM only |

Position cache invalidates on any file edit. Body cache is shared across cursor positions that resolve to the same function.

### Vim quality-of-life

| Mapping | Purpose |
|---|---|
| `<leader>cc` / `<leader>cd` | Callers / callees popup |
| `<leader>cs` | Summarize symbol under cursor |
| `<leader>cp` | Free-form prompt to LLM; "in this {file,method,line}," prefixes auto-rewrite to a `Context:` line so the LLM gets the reference (no IPC with the user's interactive Claude session) |
| `<leader>c{f,l,m}` | Yank repo-relative `file` / `file:line` / `file methodname` to system clipboard |

The yank/prompt features are intentionally Vimscript-only — they have no engine side. They exist because Claude Code (running in a sibling terminal) has no IPC for an external editor to inject prompts; copying a reference and pasting is the pragmatic substitute.

### Why no caching at the call-graph layer (yet)

The original design called for caching keyed on `(file, func, git-sha)`. In practice the LSP indexing race (handled by retry loops) and the source cache inside `walkCallees` covered most of the perf gap. A persistent cache may still be worthwhile for very large projects but hasn't been built.

## Prior Art

- **go-callvis** — full-program call graph visualizer using `go/callgraph`. Heavy but good reference for analysis.
- **vim-go** — has `:GoCallers` wrapping `guru`. Shows the Vim integration pattern (now dated).
- **VS Code Go extension** — already has "Show Call Hierarchy" (`Shift+Alt+H`). Good reference for gopls integration.
