# delphi-lsp

A small, dependency-free Delphi/Object Pascal language server written in Go. It speaks LSP over stdio and works with Neovim's built-in client, it is on development , i am using gpt-5.6-terra for this project and maintence will be chalenge.

## Included

- Multiple workspace directories (`workspaceFolders` and folder changes)
- Project indexing for `.pas`, `.dpr`, and `.dpk` files
- Directive-aware parsing for `{$IFDEF}`, `{$IFNDEF}`, `{$ELSE}`, and `{$ENDIF}`
- Document/workspace symbols, completion, hover, definition, references, and diagnostics
- Warnings for unused routine parameters, highlighted at their declarations and
  tagged as unnecessary for editor dimming. Reads, assignments, and passing a
  parameter to another routine count as use. Nested routines respect parameter
  and local-variable scope; declarations without bodies are excluded.
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

## Performance checks

Diagnostic checks share one token stream per parse. Background indexing uses
up to four workers, bounded by Go's CPU parallelism setting, and concurrent
requests for the same file share its in-progress load. Symbol index updates
remove each distinct name once, avoiding repeated scans for overloaded names.

Run the synthetic benchmarks with `go test ./internal/lsp -run '^$' -bench . -benchmem`.
To measure a private Pascal unit without copying it into this repository, use
PowerShell:

```powershell
$env:DELPHI_LSP_BENCH_FILE = 'Z:\millenium\Eventos\ExecEventoB.pas'
go test ./internal/lsp -run '^TestExternalUnitSnapshot$' -v
go test ./internal/lsp -run '^$' -bench '^BenchmarkParseExternalUnit$' -benchmem -benchtime=3x -count=3
```

The snapshot prints a hash of the parsed document to compare behavior across
revisions. Benchmarks measure parsing and allocations, excluding file reads;
allocated bytes are cumulative per parse, not peak resident memory.

## Scope

This is a practical starter server, not a Delphi compiler. It indexes declarations with a lightweight parser. The next natural extension is a full AST and compiler-compatible conditional-symbol configuration.

Diagnostics also validate unit headings, required sections and their order,
duplicate sections, uses-clause placement and syntax, nested block closures,
routine bodies in the interface section, and the final `end.`. Namespaced unit
names, hint directives, and legacy `begin ... end.` initialization are supported.
Classic local `var` declarations in routines are checked for missing semicolons,
including the final declaration before `begin` and multiline declarations.
Diagnostics also report unclosed string literals and missing or incorrect
semicolon separators between routine parameter groups. Empty strings, escaped
apostrophes, Delphi multiline strings, comma-separated names within one parameter
group, and procedural parameter types are supported.
These checks follow [Programs and Units (Delphi)](https://docwiki.embarcadero.com/RADStudio/Florence/en/Programs_and_Units_%28Delphi%29).

The checkers validate statement separators and unit structure, not the complete
Delphi grammar, declaration semantics, or interface/implementation signature
matching. Conditional compilation currently checks the
first branch without evaluating compiler symbols; assembly bodies are skipped.
Unused-parameter warnings follow the same conditional-compilation behavior.
Assembly and implicit `inherited` calls suppress warnings because they may use
parameters without explicitly naming them. Incomplete routine bodies are skipped.
