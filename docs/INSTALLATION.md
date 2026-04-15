[← Back to README](../README.md)

# Installation

- [Homebrew (macOS / Linux)](#homebrew-macos--linux)
- [Windows](#windows)
- [Install from source (macOS / Linux)](#install-from-source-macos--linux)
- [Download binary (all platforms)](#download-binary-all-platforms)
- [Requirements](#requirements)
- [Environment Variables](#environment-variables)
- [Windows Config Paths](#windows-config-paths)

---

## Homebrew (macOS / Linux)

```bash
brew install gentleman-programming/tap/mneme
```

Upgrade to latest:

```bash
brew update && brew upgrade mneme
```

> **Migrating from the old `engram` formula?** If you installed before the rename, uninstall first, then reinstall:
> ```bash
> brew uninstall engram 2>/dev/null; brew install gentleman-programming/tap/mneme
> ```

---

## Windows

**Option A: Install via `go install` (recommended for technical users)**

If you have Go installed, this is the cleanest and most trustworthy path — the binary is compiled on your machine from source, so no antivirus will flag it:

```powershell
go install github.com/Edcko/Mneme/cmd/engram@latest
# Binary goes to %GOPATH%\bin\engram.exe (typically %USERPROFILE%\go\bin\)
# Rename to mneme.exe for the new binary name:
Rename-Item "$env:GOPATH\bin\engram.exe" "$env:GOPATH\bin\mneme.exe"
```

Ensure `%GOPATH%\bin` (or `%USERPROFILE%\go\bin`) is on your `PATH`.

**Option B: Build from source**

```powershell
git clone https://github.com/Edcko/Mneme.git
cd Mneme
go build -o mneme.exe ./cmd/engram
# Binary is mneme.exe in the current directory

# Optional: install to GOPATH/bin
go install ./cmd/engram
Rename-Item "$env:GOPATH\bin\engram.exe" "$env:GOPATH\bin\mneme.exe"

# Optional: build with version stamp (otherwise `mneme version` shows "dev")
$v = git describe --tags --always
go build -ldflags="-X main.version=local-$v" -o mneme.exe ./cmd/engram
```

**Option C: Download the prebuilt binary**

1. Go to [GitHub Releases](https://github.com/Edcko/Mneme/releases)
2. Download `mneme_<version>_windows_amd64.zip` (or `arm64` for ARM devices)
3. Extract `mneme.exe` to a folder in your `PATH` (e.g. `C:\Users\<you>\bin\`)

```powershell
# Example: extract and add to PATH (PowerShell)
Expand-Archive mneme_*_windows_amd64.zip -DestinationPath "$env:USERPROFILE\bin"
# Add to PATH permanently (run once):
[Environment]::SetEnvironmentVariable("Path", "$env:USERPROFILE\bin;" + [Environment]::GetEnvironmentVariable("Path", "User"), "User")
```

> **Antivirus false positives on prebuilt binaries**
>
> Windows Defender and other antivirus tools (ESET, Brave's built-in scanner) have flagged some
> engram prebuilt releases as malware (`Trojan:Script/Wacatac.H!ml` or similar). This is a
> **heuristic false positive**. The binary is built reproducibly from the public source code
> via GoReleaser and contains no malicious code.
>
> **Why does this happen?** Prebuilt binaries from small open-source projects are unsigned (code
> signing certificates cost hundreds of dollars per year). Many AV engines automatically flag
> unsigned executables from unknown publishers, especially recently compiled Go binaries. The
> same alert has been observed on Claude Code's own MSIX installer, which confirms this is an
> AV heuristic issue, not a code problem.
>
> **Maintainer stance:** We will not pay for a code signing certificate at this time. This is a
> distribution trust problem, not a security problem. The source code is fully auditable.
>
> **Recommended workaround:** Technical Windows users should prefer **Option A (`go install`)** or
> **Option B (build from source)**. Binaries you compile locally will not trigger AV alerts because
> they originate from your own machine.

> **Other Windows notes:**
> - Data is stored in `%USERPROFILE%\.engram\engram.db`
> - Override with `ENGRAM_DATA_DIR` environment variable
> - All core features work natively: CLI, MCP server, TUI, HTTP API, Git Sync
> - No WSL required for the core binary — it's a native Windows executable

---

## Install from source (macOS / Linux)

```bash
git clone https://github.com/Edcko/Mneme.git
cd Mneme
go build -o mneme ./cmd/engram

# Optional: install to GOPATH/bin (binary will be named 'engram' — rename after)
go install ./cmd/engram && mv ~/go/bin/engram ~/go/bin/mneme

# Optional: build with version stamp (otherwise `mneme version` shows "dev")
go build -ldflags="-X main.version=local-$(git describe --tags --always)" -o mneme ./cmd/engram
```

---

## Download binary (all platforms)

Grab the latest release for your platform from [GitHub Releases](https://github.com/Edcko/Mneme/releases).

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `mneme_<version>_darwin_arm64.tar.gz` |
| macOS (Intel) | `mneme_<version>_darwin_amd64.tar.gz` |
| Linux (x86_64) | `mneme_<version>_linux_amd64.tar.gz` |
| Linux (ARM64) | `mneme_<version>_linux_arm64.tar.gz` |
| Windows (x86_64) | `mneme_<version>_windows_amd64.zip` |
| Windows (ARM64) | `mneme_<version>_windows_arm64.zip` |

---

## Requirements

- **Go 1.25+** to build from source (not needed if installing via Homebrew or downloading a binary)
- That's it. No runtime dependencies.

The binary includes SQLite (via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — pure Go, no CGO). Works natively on **macOS**, **Linux**, and **Windows** (x86_64 and ARM64).

---

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `ENGRAM_DATA_DIR` | Data directory | `~/.engram` (Windows: `%USERPROFILE%\.engram`) |
| `ENGRAM_PORT` | HTTP server port | `7437` |

---

## Windows Config Paths

When using `mneme setup`, config files are written to platform-appropriate locations:

| Agent | macOS / Linux | Windows |
|-------|---------------|---------|
| OpenCode | `~/.config/opencode/` | `%APPDATA%\opencode\` |
| Gemini CLI | `~/.gemini/` | `%APPDATA%\gemini\` |
| Codex | `~/.codex/` | `%APPDATA%\codex\` |
| Claude Code | Managed by `claude` CLI | Managed by `claude` CLI |
| VS Code | `.vscode/mcp.json` (workspace) or `~/Library/Application Support/Code/User/mcp.json` (user) | `.vscode\mcp.json` (workspace) or `%APPDATA%\Code\User\mcp.json` (user) |
| Antigravity | `~/.gemini/antigravity/mcp_config.json` | `%USERPROFILE%\.gemini\antigravity\mcp_config.json` |
| Data directory | `~/.engram/` | `%USERPROFILE%\.engram\` |
