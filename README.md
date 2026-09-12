# delphi-lsp

A small, dependency-free Delphi/Object Pascal language server written in Go. It speaks LSP over stdio and works with Neovim's built-in client, it is on development , i am using gpt-5.6-terra for this project and maintence will be chalenge.

## Included

- Multiple workspace directories (`workspaceFolders` and folder changes)
- Project indexing for `.pas`, `.dpr`, and `.dpk` files
- Directive-aware parsing for `{$IFDEF}`, `{$IFNDEF}`, `{$ELSE}`, and `{$ENDIF}`
- Document/workspace symbols, completion, hover, definition, references, and diagnostics
- Error diagnostics for missing `;` separators in executable Pascal statements,
  including multiline commands and nested control-flow blocks. Comments and
  strings are ignored, and optional separators before `end` / `until` and
  `if` / `else` syntax are respected.

## Build

```sh
go build -o delphi-lsp ./cmd/delphi-lsp
```

## Neovim (0.11+)

```lua
vim.lsp.config('delphi_lsp', {
  cmd = { '/absolute/path/to/delphi-lsp' },
  filetypes = { 'pascal', 'delphi' },
  root_markers = { '.git', '*.dproj', '*.dpr' },
})
vim.lsp.enable('delphi_lsp')
```

For `nvim-lspconfig`, use the same `cmd`, `filetypes`, and `root_dir` options.

## Scope

This is a practical starter server, not a Delphi compiler. It indexes declarations with a lightweight parser. The next natural extension is a full AST and compiler-compatible conditional-symbol configuration.

The semicolon checker validates statement separators, not the complete Delphi
grammar or declaration syntax. Conditional compilation currently checks the
first branch without evaluating compiler symbols; assembly bodies are skipped.
