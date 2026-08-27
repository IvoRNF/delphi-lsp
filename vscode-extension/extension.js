const path = require('path');
const vscode = require('vscode');
const { LanguageClient, TransportKind } = require('vscode-languageclient/node');

let client;

function expandWorkspacePath(value) {
  if (!value || path.isAbsolute(value)) {
    return value;
  }
  const folder = vscode.workspace.workspaceFolders?.[0];
  return folder ? path.join(folder.uri.fsPath, value) : value;
}

function bundledServerPath(context) {
  const platformDirectory = process.platform === 'win32' && process.arch === 'x64'
    ? 'win32-x64'
    : undefined;
  return platformDirectory
    ? context.asAbsolutePath(path.join('bin', platformDirectory, 'delphi-lsp.exe'))
    : undefined;
}

async function getLspConfigPath(context, settings) {
  const configuredPath = expandWorkspacePath(settings.get('configPath'));
  if (configuredPath) {
    return configuredPath;
  }

  const config = {
    project: expandWorkspacePath(settings.get('project', '')),
    delphiDir: expandWorkspacePath(settings.get('delphiDir', '')),
    searchPaths: settings.get('searchPaths', []).map(expandWorkspacePath),
    includePaths: settings.get('includePaths', []).map(expandWorkspacePath)
  };
  const hasSettings = config.project || config.delphiDir || config.searchPaths.length || config.includePaths.length;
  if (!hasSettings) {
    return undefined;
  }

  await vscode.workspace.fs.createDirectory(context.globalStorageUri);
  const generatedConfig = vscode.Uri.joinPath(context.globalStorageUri, 'delphi-lsp.json');
  await vscode.workspace.fs.writeFile(
    generatedConfig,
    Buffer.from(JSON.stringify(config, null, 2), 'utf8')
  );
  return generatedConfig.fsPath;
}

async function createClient(context) {
  const settings = vscode.workspace.getConfiguration('delphiLsp');
  const configuredPath = expandWorkspacePath(settings.get('serverPath'));
  const command = configuredPath || bundledServerPath(context);
  if (!command) {
    throw new Error('No bundled Delphi LSP server exists for this platform. Set delphiLsp.serverPath to a compatible executable.');
  }

  const args = [...settings.get('serverArgs', [])];
  const configPath = await getLspConfigPath(context, settings);
  if (configPath) {
    args.push('--config', configPath);
  }

  const serverOptions = {
    command,
    args,
    transport: TransportKind.stdio
  };
  const clientOptions = {
    documentSelector: [
      { scheme: 'file', language: 'delphi' },
      { scheme: 'file', language: 'pascal' },
      // Other Pascal extensions commonly assign this language ID to .pas/.dpr.
      { scheme: 'file', language: 'objectpascal' }
    ],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.{pas,dpr,dpk,dproj,inc,pp,lpr}')
    }
  };
  return new LanguageClient('delphiLsp', 'Delphi LSP', serverOptions, clientOptions);
}

async function activate(context) {
  const start = async () => {
    if (client) {
      await client.stop();
    }
    client = await createClient(context);
    await client.start();
  };
  try {
    await start();
    context.subscriptions.push(vscode.workspace.onDidChangeConfiguration(async event => {
      if (event.affectsConfiguration('delphiLsp')) {
        try {
          await start();
        } catch (error) {
          vscode.window.showErrorMessage(`Delphi LSP could not restart: ${error.message}`);
        }
      }
    }));
  } catch (error) {
    vscode.window.showErrorMessage(`Delphi LSP could not start: ${error.message}`);
  }
}

async function deactivate() {
  if (client) {
    await client.stop();
  }
}

module.exports = { activate, deactivate };
