# Vido Tunnel

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5.3-7952B3?style=flat&logo=bootstrap)](https://getbootstrap.com)
[![Security](https://img.shields.io/badge/Security-Sandboxed%20%26%20Rate--Limited-success?style=flat)]()

**Vido Tunnel** is a lightweight, secure, and self-contained Go web application designed for private directory browsing, file downloading, folder streaming as ZIP archives, folder renaming, and local application control (`Vidoveo.exe` & TCP Port `7788`).

The entire application—including HTML templates, Bootstrap 5.3 CSS/JS, and Bootstrap Icons—is compiled directly into a single Windows executable with zero runtime external asset dependencies.

---

## Key Features

### 📁 1. Sandboxed Directory Explorer
- **Folder Navigation**: Interactive breadcrumb trail and subfolder navigation.
- **On-the-Fly ZIP Streaming**: Compresses and streams entire directory trees on-the-fly directly to the HTTP response stream without writing temporary ZIP files to disk or overloading RAM.
- **Single File Downloads**: Serves files with HTTP Range request support (essential for video seek/playback and resuming large downloads).
- **Safe Folder Renaming**: Rename directories in place with real-time modal feedback and strict server-side validation.
- **Live Search Filter**: Instant client-side search filtering across files and directories.
- **Zero File Upload Policy**: Strictly read-only on files. No upload handlers exist in the codebase.

### 🎮 2. Application & Port Control Panel
- **`Vidoveo.exe` Supervisor**: Checks if `C:\Vidoveo\Vidoveo.exe` is running, shows its Process ID (PID), and allows authorized users to start or stop the application remotely. When started, it opens in the **background minimized** so it does not interrupt active windows or pop up on top.
- **Port 7788 Health Monitor**: Probes local TCP socket `127.0.0.1:7788` with live status feedback.
- **Live Auto-Polling**: Real-time status updates every 3 seconds with animated status indicators.

### 🛡️ 3. Built-In Security Hardening
- **Secret Key Authentication**: Constant-time key comparison (`crypto/subtle.ConstantTimeCompare`) eliminates side-channel timing attacks.
- **Cryptographic HMAC-SHA256 Sessions**: Issues tamper-resistant, timestamped session cookies (`HttpOnly`, `SameSite=Strict`).
- **IP Rate Limiting & Brute-Force Defense**: In-memory rate limiter locks out remote IPs after 5 consecutive failed login attempts.
- **Windows Path Traversal Mitigation**:
  - Normalizes and jails all paths within the configured root directory.
  - Blocks `..` traversal, Alternate Data Streams (`file.txt::$DATA` or `file:stream`), null bytes, and symlink escapes.
  - Rejects Windows reserved device names (`CON`, `PRN`, `AUX`, `NUL`, `COM1-9`, `LPT1-9`).
- **POST-Based Action Architecture**: All sensitive operations (downloads, ZIP generation, renames, process controls) use HTTP `POST` requests rather than exposing paths or actions in URL query strings.
- **Security HTTP Headers**: Includes `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and `Content-Security-Policy`.

---

## Architecture & Directory Structure

```text
vido-tunnel/
├── main.go                       # Entry point, flag/env config, routing & graceful shutdown
├── go.mod                        # Go module definition
├── internal/
│   ├── config/
│   │   └── config.go             # Configuration parser (flags, environment variables, defaults)
│   ├── auth/
│   │   ├── auth.go               # Constant-time auth & HMAC session management
│   │   ├── auth_test.go          # Auth & cookie tampering unit tests
│   │   └── ratelimit.go          # In-memory IP rate limiter
│   ├── fs/
│   │   ├── sandbox.go            # Sandboxed path jail & Windows path validation
│   │   ├── sandbox_test.go       # Path traversal & rename unit tests
│   │   ├── explorer.go           # Directory walker, breadcrumbs & metadata formatting
│   │   └── archiver.go           # Streaming on-the-fly ZIP generator
│   ├── process/
│   │   ├── manager.go            # Vidoveo.exe process supervisor & Port 7788 monitor
│   │   └── process_test.go       # Process manager & port probe unit tests
│   └── handlers/
│       ├── handlers.go           # HTTP handlers (Login, Browse, Download, Zip, Rename, Control)
│       ├── handlers_test.go      # Integration tests for all endpoints
│       └── middleware.go         # Security headers, logging & session enforcement
└── web/
    ├── embed.go                  # Go embed.FS for templates & static assets
    ├── templates/
    │   ├── base.html             # Base layout, navbar tabs & dark theme
    │   ├── login.html            # Authentication card with secret key input
    │   ├── explorer.html         # Directory browser with POST actions & rename modal
    │   └── control.html          # Process & Port 7788 Control Panel
    └── static/
        ├── css/
        │   ├── bootstrap.min.css # Embedded Bootstrap 5.3 CSS
        │   ├── bootstrap-icons.min.css # Embedded Bootstrap Icons CSS
        │   ├── custom.css        # Dark mode theme & glowing status badges
        │   └── fonts/            # Embedded icon webfonts (offline support)
        └── js/
            ├── bootstrap.bundle.min.js # Embedded Bootstrap 5.3 JS
            ├── app.js            # Directory explorer POST handlers & rename modal logic
            └── control.js        # Control panel real-time AJAX poller & action triggers
```

---

## Getting Started

### 1. Prerequisites
- **Go 1.22+** installed on your system.

### 2. Build Executable
Compile the application into a single executable binary:
```powershell
go build -o vido-tunnel.exe .
```

### 3. Run the Server
Launch the server with custom flags or default settings:
```powershell
.\vido-tunnel.exe -root "C:\Users\sk\Videos\Vidoveo" -key "yourSecretKey123" -port 8080 -vidoveo-path "C:\Vidoveo\Vidoveo.exe" -vidoveo-port 7788
```

Open your browser and navigate to:
```text
http://localhost:8080
```

---

## Configuration

Settings can be specified using either command-line flags or environment variables:

| Flag | Environment Variable | Default Value | Description |
| :--- | :--- | :--- | :--- |
| `-root` | `VIDO_ROOT_DIR` | `C:\Users\sk\Videos\Vidoveo` | Root directory path to browse and manage |
| `-key` | `VIDO_SECRET_KEY` | `vido-secret-key-2026` | Secret key required for authentication |
| `-port` | `VIDO_PORT` | `8080` | HTTP port the server listens on |
| `-vidoveo-path` | `VIDOVEO_PATH` | `C:\Vidoveo\Vidoveo.exe` | Path to the `Vidoveo.exe` binary |
| `-vidoveo-port` | `VIDOVEO_PORT` | `7788` | Local TCP port monitored for Vidoveo |

---

## Running Automated Tests

Run the full test suite with verbose output:
```powershell
go test -v ./...
```

The test suite validates:
- Secret key verification and HMAC session cookie tamper resistance.
- IP rate limiter lockout and expiry.
- Path traversal attack vectors (`../`, `..\..`, `/../`, colons, null bytes, device names).
- Safe folder rename operations.
- HTTP Range and single-file downloads.
- Streaming ZIP generation.
- TCP port checking and process status probing.

---

## Exposing to the Internet (Recommended Setup)

When hosting this application for remote users over the internet, avoid opening raw router ports. Instead, use a secure tunnel service that provides automatic TLS/HTTPS encryption and DDoS protection:

### Option A: Cloudflare Tunnel (`cloudflared`)
1. Download [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/).
2. Run a quick tunnel pointing to your local port:
   ```powershell
   cloudflared tunnel --url http://localhost:8080
   ```
3. Share the generated HTTPS URL (e.g., `https://xxxx.trycloudflare.com`) with authorized users.

### Option B: Tailscale Funnel
1. Install [Tailscale](https://tailscale.com).
2. Enable funnel on port 8080:
   ```powershell
   tailscale funnel 8080
   ```

---

## License

MIT License. See [LICENSE](LICENSE) for details if applicable.
