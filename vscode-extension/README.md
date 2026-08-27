# Delphi LSP for VS Code

This extension provides Delphi/Object Pascal language features through the bundled `delphi-lsp` language server.

## Included features

- Workspace and document symbols
- Completion and hover information
- Go to definition and find references
- Diagnostics

## Configuration

All LSP settings are available in VS Code's **Settings** panel under **Delphi LSP**. The Windows x64 server is bundled. For another platform or a custom build, set `delphiLsp.serverPath` to its executable.

You can configure a `delphiLsp.configPath` to use an existing `delphi-lsp.json` file, or configure these fields directly in VS Code: `delphiLsp.project`, `delphiLsp.delphiDir`, `delphiLsp.searchPaths`, and `delphiLsp.includePaths`. Direct settings generate the server configuration automatically. Changes to Delphi LSP settings restart the language server.

Relative setting paths are resolved from the first workspace folder.
