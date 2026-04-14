# callgraph

A CLI tool and Vim/Neovim plugin that shows the **upward call graph** (callers) of a function. Place your cursor anywhere inside a function body, hit one keystroke, and see every call path that leads to it.

Supports **Go**, **Python**, and **Rust**. Language is auto-detected from the file extension.

```
main (main)
  |__ startRouter (main)
        |__ GetRouter (router)
              |__ addOrderRoutes (routes)
                    |__ PlaceOrder (handlers)
                          |__ CreateOrder (orders)
TestPlaceOrder (orders_test)
  |__ CreateOrder (orders)
```

## How it works

The tool communicates with a language server (LSP) to query `callHierarchy/incomingCalls` recursively. It starts the appropriate LSP server automatically:

| Language | LSP Server | Project Root Marker |
|----------|------------|---------------------|
| Go       | `gopls`    | `go.mod`            |
| Python   | `pyright-langserver` or `pylsp` | `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt` |
| Rust     | `rust-analyzer` | `Cargo.toml`    |

## Installation

### Prerequisites

You need Go 1.22+ and the LSP server for your language:

**Go:**
```sh
go install golang.org/x/tools/gopls@latest
```

**Python** (one of):
```sh
sudo npm install -g pyright
```

**Rust:**
```sh
rustup component add rust-analyzer
```

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

Ensure `$GOPATH/bin` (usually `~/go/bin`) is in your `$PATH`.

### Install the Vim/Neovim plugin

**vim-plug:**
```vim
Plug 'madhanganesh/callgraph', { 'rtp': 'vim' }
```

**lazy.nvim:**
```lua
{ 'madhanganesh/callgraph', config = function() vim.opt.rtp:append(vim.fn.stdpath('data') .. '/lazy/callgraph/vim') end }
```

**Manual:**
Copy the `vim/` directory contents into your Vim runtime path:
```sh
cp -r vim/plugin/ ~/.vim/plugin/
cp -r vim/autoload/ ~/.vim/autoload/
```

## Usage

### CLI

```
callgraph --file=<path> --line=<n> [--col=<n>] [--depth=<n>] [--format=json|tree]
```

| Flag       | Default | Description                          |
|------------|---------|--------------------------------------|
| `--file`   | required | Absolute path to the source file    |
| `--line`   | required | Line number (1-based)               |
| `--col`    | `1`     | Column number (1-based)              |
| `--depth`  | `5`     | Max caller traversal depth           |
| `--format` | `tree`  | Output format: `json` or `tree`      |

The cursor can be **anywhere inside a function body** -- the tool finds the enclosing function automatically.

**Examples:**

```sh
# Go
callgraph --file=order/order.go --line=15

# Python
callgraph --file=app/services/order.py --line=42

# Rust
callgraph --file=src/handlers/order.rs --line=30

# JSON output (for editor integrations)
callgraph --file=main.go --line=10 --format=json
```

### Vim / Neovim

| Command       | Mapping      | Description       |
|---------------|--------------|-------------------|
| `:CallGraph`  | `<leader>cc` | Show call graph   |

Inside the popup:

| Key     | Action                     |
|---------|----------------------------|
| `j`/`k` | Navigate up/down          |
| `Enter` | Jump to the selected caller|
| `q`/`Esc`| Close the popup           |

Works in Vim 8.2+ (popup window) and Neovim 0.9+ (floating window).

### Configuration

```vim
" Custom binary path (default: 'callgraph')
let g:callgraph_binary = '/path/to/callgraph'

" Custom keybinding (default: <leader>cc)
nmap <leader>cg <Plug>(callgraph)
```

## JSON output format

When using `--format=json`, the output is a tree of caller nodes:

```json
{
  "name": "CreateOrder",
  "pkg": "orders",
  "file": "/project/order/order.go",
  "line": 10,
  "callers": [
    {
      "name": "PlaceOrder",
      "pkg": "handlers",
      "file": "/project/handlers/order.go",
      "line": 25,
      "callers": [...]
    }
  ]
}
```

The `pkg` field shows a display-friendly relative path from the project root:
- **Go:** dotted package path (e.g. `api.v1`)
- **Python:** dotted module path (e.g. `app.services`)
- **Rust:** `::` separated module path (e.g. `handlers::order`)

## Project structure

```
callgraph/
├── main.go                 # CLI entry point + flag parsing
├── internal/
│   ├── lang/               # Language abstraction layer
│   │   ├── lang.go         #   Language interface + auto-detection
│   │   ├── golang.go       #   Go: gopls, go/ast, go.mod
│   │   ├── python.go       #   Python: pyright/pylsp, regex
│   │   └── rust.go         #   Rust: rust-analyzer, regex
│   ├── lsp/                # LSP client (language-agnostic)
│   │   ├── client.go       #   JSON-RPC over stdin/stdout
│   │   └── types.go        #   LSP protocol types
│   ├── graph/
│   │   └── traverse.go     #   Call tree building via LSP
│   └── output/
│       └── formatter.go    #   JSON + ASCII tree formatters
├── vim/                    # Vim/Neovim plugin
│   ├── plugin/
│   │   └── callgraph.vim
│   └── autoload/
│       └── callgraph.vim
└── testdata/               # Test fixtures
```

## Architecture

```
┌──────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│  Vim/Neovim      │────>│  callgraph (CLI)      │────>│ gopls           │
│  :CallGraph      │     │                       │     │ pyright / pylsp │
└──────────────────┘     │  Language interface    │     │ rust-analyzer   │
                         │  auto-detects from ext │     └─────────────────┘
                         └──────────────────────┘
```

The CLI is the single source of logic. The Vim plugin is a thin wrapper that captures cursor position, shells out to the CLI, parses the JSON response, and renders a popup.

## Adding a new language

Implement the `Language` interface in `internal/lang/`:

```go
type Language interface {
    FindRoot(file string) string
    RelPkg(file, root string) string
    EnclosingFunc(src []byte, line int) (funcLine, funcCol int)
    LSPCommand() []string
    LanguageID() string
}
```

Then add the file extension mapping in `Detect()` in `lang.go`.

## License

MIT
