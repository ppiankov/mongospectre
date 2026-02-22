# mongospectre VS Code Extension

Surface `mongospectre check` findings directly in your editor.

## Features

- Inline diagnostics on collection references in Go, Python, JavaScript, and TypeScript.
- Hover metadata for collections (document count and index usage details).
- Quick fix to append ignore rules into `.mongospectreignore`.
- Status bar summary (`mongospectre: N issues`).
- Debounced auto-refresh on save.

## Requirements

- `mongospectre` CLI installed and available in `PATH`, or configure `mongospectre.binaryPath`.
- Access to a MongoDB URI configured in extension settings.

## Configuration

```json
{
  "mongospectre.uri": "mongodb://localhost:27017",
  "mongospectre.database": "myapp",
  "mongospectre.autoRefresh": true,
  "mongospectre.binaryPath": "mongospectre"
}
```

## Commands

- `mongospectre: Refresh Diagnostics`

## Install From Marketplace

Extension ID: `ppiankov.mongospectre`

```bash
code --install-extension ppiankov.mongospectre
```

Or install from the VS Code Extensions view by searching for `mongospectre`.

## Build And Install VSIX

```bash
cd vscode-mongospectre
npm install
npm run package:vsix
```

Install via VS Code:

1. Open Extensions view.
2. Click `...` -> `Install from VSIX...`.
3. Select the generated `.vsix` file.

## Publish to Marketplace

```bash
cd vscode-mongospectre
npm install
npm run publish:marketplace
```

Requires `VSCE_PAT` token configured for the publisher.
