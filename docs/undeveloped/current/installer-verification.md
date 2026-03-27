# SoloDawn Windows Installer - Verification Plan (Lightweight)

## Overview

The lightweight installer ships only `server.exe` and `tray.exe`. Node.js, Git, and npm are expected to be pre-installed on the user's system and reachable via PATH.

## Test Environment Requirements

- Windows 10/11 machine with Node.js (>=18), Git, and npm installed
- Admin privileges
- Port 23456 available

## Verification Checklist

### 1. Installation

- [ ] Run `SoloDawn-Setup-v{version}.exe`
- [ ] Installation completes without errors
- [ ] Desktop shortcut created (if selected)
- [ ] Start Menu folder created

### 2. Installed Files

- [ ] `{app}\server.exe` exists and is executable
- [ ] `{app}\tray.exe` exists and is executable
- [ ] `{app}\.env` generated with `SOLODAWN_ENCRYPTION_KEY` (32 bytes) and `SOLODAWN_API_TOKEN`
- [ ] `.env` values are non-empty and correctly formatted

### 3. System Prerequisites Detection

Verify the installer's post-install check detects tools already on the system:

- [ ] `node --version` resolves via PATH
- [ ] `git --version` resolves via PATH
- [ ] `npm --version` resolves via PATH

If any prerequisite is missing, the installer should warn the user.

### 4. Server Startup and Health

- [ ] Launch tray app (or start `server.exe` directly)
- [ ] Server binds to `http://127.0.0.1:23456`
- [ ] `GET /healthz` returns HTTP 200
- [ ] `GET /readyz` returns HTTP 200 (DB initialized, asset dir present)
- [ ] Web UI loads at `http://127.0.0.1:23456`

### 5. Runtime Settings Page

- [ ] Navigate to Settings > Runtime
- [ ] Node.js detected and version displayed
- [ ] Git detected and version displayed
- [ ] npm detected and version displayed
- [ ] Refresh button re-detects prerequisites

### 6. System Tray Lifecycle

- [ ] Tray icon appears on launch
- [ ] Open SoloDawn: opens browser to web UI
- [ ] Start/Stop Server: controls `server.exe` process
- [ ] Quit: stops server and exits tray app

### 7. Update (Overwrite) Test

- [ ] Run newer installer over existing installation
- [ ] `.env` file preserved (encryption key not regenerated)
- [ ] Database preserved
- [ ] Server starts successfully after update

### 8. Uninstall Test

- [ ] Run uninstaller from Add/Remove Programs
- [ ] `server.exe` and `tray.exe` processes killed
- [ ] Installation directory removed
- [ ] Desktop shortcut and Start Menu folder removed

### 9. Silent Install Test

```powershell
SoloDawn-Setup-v{version}.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART
```

- [ ] Installs without any UI
- [ ] `.env` generated
- [ ] Server accessible after installation

## Known Limitations

1. **No code signing**: Windows SmartScreen may warn on first run. Click "More info" then "Run anyway".
2. **System prerequisites required**: Node.js, Git, and npm must be installed separately before running SoloDawn.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| SmartScreen blocks installer | Not code-signed | Click "More info" then "Run anyway" |
| Server won't start | Port 23456 in use | `netstat -ano \| findstr :23456`, kill conflicting process |
| "Node.js not found" in Runtime Settings | Node.js not on PATH | Install Node.js and restart tray app |
| Database errors | Missing encryption key | Verify `SOLODAWN_ENCRYPTION_KEY` in `{app}\.env` |
| `/readyz` fails | DB not initialized | Run `pnpm run prepare-db` or restart server (auto-migrates) |
