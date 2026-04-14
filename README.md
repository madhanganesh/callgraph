# callgraph

A CLI tool and Vim/Neovim plugin for navigating and understanding code:

- **Upward call graph** (callers / incoming) — *who calls this function?*
- **Downward call graph** (callees / outgoing) — *what does this method do?*, with API 🌐 / DB 🛢️ / thread 🧵 classification.
- **LLM summary** — *give me a 2-3 sentence description of the symbol under cursor.*
- **Free-form prompt** — *ask anything about this file/method/line and get the answer in a popup.*
- **Yank reference** — copy the current `file`, `file:line`, or `file method` to the clipboard for pasting into another tool.

Supports **Go**, **Python**, and **Rust**. Language is auto-detected from the file extension.

```
main (main)
  |__ startRouter (main)
        |__ GetRouter (router)
              |__ addOrderRoutes (routes)
                    |__ PlaceOrder (handlers)
                          |__ CreateOrder (orders)
```

## How it works

The tool talks to a language server (LSP) to query `callHierarchy/incomingCalls`, `callHierarchy/outgoingCalls`, and `textDocument/implementation`. The appropriate LSP server starts automatically:

| Language | LSP Server | Project Root Marker |
|----------|------------|---------------------|
| Go       | `gopls`    | `go.mod`            |
| Python   | `pyright-langserver` or `pylsp` | `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt` |
| Rust     | `rust-analyzer` | `Cargo.toml`    |

## Installation

### Prerequisites

You need Go 1.22+ and the LSP server for your language:

**Go:** `go install golang.org/x/tools/gopls@latest`
**Python:** `npm install -g pyright` (or `pip install python-lsp-server`)
**Rust:** `rustup component add rust-analyzer`

For the **summarize** and **prompt** features you also need an LLM CLI on your `$PATH`. Defaults to `claude -p` (Claude Code). Anything that reads a prompt from stdin and prints a response works — see [LLM CLI options](#llm-cli-options) below.

### Install the CLI

```sh
go install github.com/madhanganesh/callgraph@latest
```

Or build from source:

```sh
git clone https://github.com/madhanganesh/callgraph.git
cd callgraph
go install .
```

### Install the Vim/Neovim plugin

**vim-plug:**
```vim
Plug 'madhanganesh/callgraph', { 'rtp': 'vim' }
```

**lazy.nvim:**
```lua
{ 'madhanganesh/callgraph', config = function()
    vim.opt.rtp:append(vim.fn.stdpath('data') .. '/lazy/callgraph/vim')
end }
```

**Manual:**
```sh
cp -r vim/plugin/  ~/.vim/plugin/
cp -r vim/autoload/ ~/.vim/autoload/
```

## Vim / Neovim mappings

All defaults respect existing maps — they only bind if the key is free.

| Mapping       | Command                  | Action |
|---------------|--------------------------|--------|
| `<leader>cc`  | `:CallGraph`             | Upward call graph (callers) of enclosing function |
| `<leader>cd`  | `:CallGraphCallees`      | Downward call graph (callees) of symbol under cursor |
| `<leader>cs`  | `:CallGraphSummarize`    | LLM summary of symbol under cursor |
| `<leader>cp`  | `:CallGraphPrompt`       | Free-form prompt → LLM response in a popup |
| `<leader>cf`  | `:CallGraphYankFile`     | Yank repo-relative file path |
| `<leader>cl`  | `:CallGraphYankFileLine` | Yank `path:line` |
| `<leader>cm`  | `:CallGraphYankFileMethod` | Yank `path method` (enclosing function name) |

Inside the call-graph / summary / picker popup:

| Key         | Action                       |
|-------------|------------------------------|
| `j` / `k`   | Navigate                     |
| `<CR>`      | Jump (graph) / pick (picker) |
| `q` / `Esc` | Close                        |

Works in Vim 8.2+ (popup) and Neovim 0.9+ (floating window).

### `<leader>cp` prompt prefixes

When the question begins with one of these phrases, the plugin rewrites it into an explicit `Context:` line so the LLM (which has no IPC with your interactive Claude session) gets the reference:

| You type | Sent to LLM |
|---|---|
| `in this file, what does it do?` | `Context: path/to/file.go\n\nwhat does it do?` |
| `in this method, why the retry?` | `Context: path/to/file.go MethodName\n\nwhy the retry?` |
| `in this line, explain` | `Context: path/to/file.go:42\n\nexplain` |
| `anything else` | sent verbatim |

### Configuration

```vim
" Custom binary path (default: 'callgraph')
let g:callgraph_binary = '/path/to/callgraph'

" LLM CLI for summarize and prompt (default: 'claude -p')
let g:callgraph_llm_cmd = 'llm -m claude-haiku-4-5'
" or:  let g:callgraph_llm_cmd = 'llm -m gpt-4o-mini'
" or:  let g:callgraph_llm_cmd = 'llm -m llama3.2'   " local Ollama
```

Override any default mapping by binding the `<Plug>` mapping yourself:

```vim
nmap <leader>x <Plug>(callgraph)
nmap <leader>X <Plug>(callgraph-callees)
" <Plug>(callgraph-summarize), <Plug>(callgraph-prompt),
" <Plug>(callgraph-yank-file), <Plug>(callgraph-yank-line),
" <Plug>(callgraph-yank-method)
```

## CLI

```
callgraph --file=<path> --line=<n> [--col=<n>] [--depth=<n>]
          [--direction=callers|callees] [--format=json|tree]

callgraph summarize --file=<path> --line=<n> [--col=<n>] [--llm-cmd='claude -p']
```

| Flag          | Default     | Description                                  |
|---------------|-------------|----------------------------------------------|
| `--file`      | required    | Source file path                             |
| `--line`      | required    | Line number (1-based)                        |
| `--col`       | `1`         | Column number (1-based)                      |
| `--depth`     | `5`         | Max traversal depth                          |
| `--direction` | `callers`   | `callers` (incoming) or `callees` (outgoing) |
| `--format`    | `tree`      | `json` or `tree`                             |
| `--llm-cmd`   | `claude -p` | (summarize only) command that reads stdin    |

For `--direction=callers` the cursor can be anywhere inside a function body. For `--direction=callees` and `summarize` the cursor must be on the **symbol** you want to inspect (a function name or call site).

**Examples:**

```sh
# Who calls compute()?
callgraph --file=main.go --line=21 --direction=callers

# What does PlaceOrder() call?
callgraph --file=handlers/order.go --line=15 --direction=callees

# Summarize the function under cursor
callgraph summarize --file=main.go --line=21 --col=6
```

## Caching (summarize)

Summarize uses a two-layer cache to avoid repeat LSP startup and LLM calls:

- **Position cache** — `(file, line, col, file-content-hash, llmCmd)` → full result. A repeat keypress on the same cursor in an unmodified file skips LSP entirely.
- **Body cache** — `(prompt-content-hash)` → LLM response. Different cursor positions that resolve to the same function body share the cached summary.

Cache location (override with `CALLGRAPH_CACHE_DIR`):
- macOS: `~/Library/Caches/callgraph/summaries/`
- Linux: `~/.cache/callgraph/summaries/`

To clear: `rm -rf ~/Library/Caches/callgraph` (or your platform's equivalent).

## LLM CLI options

| Provider | Cost / summary | Setup |
|---|---|---|
| Claude Code (`claude -p`) | Counts against your Claude plan limits | Default; needs `claude` CLI logged in |
| `llm` + Anthropic Haiku | ~$0.0003 | `pipx install llm && llm install llm-anthropic && llm keys set anthropic` |
| `llm` + OpenAI mini | ~$0.0001 | `pipx install llm && llm keys set openai` |
| `llm` + Ollama | $0 (local) | `brew install ollama && ollama pull llama3.2 && llm install llm-ollama` |

Then `let g:callgraph_llm_cmd = 'llm -m claude-haiku-4-5'` etc.

## JSON output

```json
{
  "name": "CreateOrder",
  "pkg": "orders",
  "file": "/project/order/order.go",
  "line": 10,
  "callers": [
    { "name": "PlaceOrder", "pkg": "handlers", "file": "...", "line": 25, "callers": [...] }
  ]
}
```

For callees, the same shape but with `callees` and (for API/DB/thread leaves) `kind` + `detail` fields. For interface methods with multiple implementations, `implementations` replaces `callees`.

## Project structure

```
callgraph/
├── main.go
├── internal/
│   ├── lang/         # Per-language: LSP cmd, EnclosingFunc, classify rules
│   ├── lsp/          # JSON-RPC LSP client
│   ├── graph/        # Caller / callee tree builder + interface picker
│   ├── classify/     # API / DB / thread Kind + icon
│   ├── output/       # JSON + ASCII tree formatters
│   └── summarize/    # LLM summary subcommand + 2-layer cache
├── vim/
│   ├── plugin/callgraph.vim     # Commands + default mappings
│   └── autoload/callgraph.vim   # All plugin logic
└── test/             # Per-language integration tests + fixtures
```

## Adding a new language

Implement the `Language` interface in `internal/lang/`:

```go
type Language interface {
    FindRoot(file string) string
    RelPkg(file, root string) string
    EnclosingFunc(src []byte, line int) (funcLine, funcCol int)
    LSPCommand() []string
    LanguageID() string
    ClassifyRules() []classify.Rule       // optional, return nil to skip
    ThreadSpawnPattern() *regexp.Regexp   // optional
}
```

Then add the file extension mapping in `Detect()` in `lang.go`.

## License

MIT
