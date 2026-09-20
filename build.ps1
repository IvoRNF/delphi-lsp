go build -o delphi-lsp.exe ./cmd/delphi-lsp;
cp delphi-lsp.exe C:\delphi-lsp\vscode-extension\bin\win32-x64\delphi-lsp.exe;
Write-Host "Copied to vscode extension."

