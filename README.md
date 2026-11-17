# Krotek Script Hub — Go Installers, Binaries & Source Archives

Standalone distribution repository containing **34 Linux Go management & installation tools** across **9 categories**.

This repository contains only the Go source code, pre-compiled static Linux binaries, installer bootstrap scripts, and source `.tar.gz` archives (no frontend/website files).

## Repository Structure

```text
.
├── scripts/                  # Go source code & bootstrap scripts by category/service
│   └── <category>/<service>/
│       ├── install.sh        # Ready-to-run POSIX bootstrap installer
│       ├── install.sh.tpl    # Installer template with host/checksum placeholders
│       └── src/              # Go module source (go.mod, main.go, modules, tui, system)
├── install/                  # Flat ready-to-run download scripts (<service>.sh)
├── binaries/                 # Static Linux ELF binaries (amd64 & arm64) + SHA-256 files
│   ├── checksums.txt         # Master SHA-256 manifest
│   └── <service>/
│       ├── krotek-<service>-linux-amd64
│       ├── krotek-<service>-linux-amd64.sha256
│       ├── krotek-<service>-linux-arm64
│       └── krotek-<service>-linux-arm64.sha256
├── archives/                 # Deterministic .tar.gz source archives by category
│   └── <category>/<service>.tar.gz
├── .gitattributes            # Enforces LF line endings for scripts/Go and binary mode for ELF/tar.gz
├── .gitignore
└── LICENSE
```

## Quick Usage

### 1. Run via Bootstrap Script
Each `install/<service>.sh` (and `scripts/<category>/<service>/install.sh`) detects the CPU architecture (`x86_64` / `arm64`), downloads the corresponding binary from `https://kroteknet.com/binaries`, verifies its SHA-256 checksum, installs `/usr/local/bin/<service>-manager`, and launches the TUI:

```bash
curl -fsSL https://kroteknet.com/i/nginx | sudo sh
# or locally from this repo:
sudo sh install/nginx.sh
```

### 2. Verify Binary Checksums Locally
```bash
cd binaries
sha256sum -c checksums.txt
```

### 3. Build from Go Source
Requires Go 1.21+ (cross-compiles cleanly with `CGO_ENABLED=0`):

```bash
cd scripts/web-server/nginx/src
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-w -s -buildid=" -o krotek-nginx-linux-amd64 .
```

### 4. Extract Source Archive
```bash
tar -xzf archives/web-server/nginx.tar.gz
```

## Included Services (34)

| Category | Count | Services |
|---|---:|---|
| `cms` | 5 | `drupal`, `ghost`, `joomla`, `moodle`, `wordpress` |
| `database` | 6 | `mariadb`, `mongodb`, `mysql`, `postgresql`, `redis`, `sqlite` |
| `email-server` | 3 | `exim`, `mailinabox`, `postfix` |
| `game-servers` | 2 | `minecraft`, `nitrox` |
| `media-server` | 7 | `emby`, `jellyfin`, `nextcloud`, `nfs`, `plex`, `samba`, `vsftpd` |
| `security` | 1 | `firewall-manager` |
| `vpn` | 2 | `openvpn`, `wireguard` |
| `web-server` | 5 | `apache`, `caddy`, `nginx`, `openlitespeed`, `openresty` |
| `webpanel` | 3 | `cyberpanel`, `hestiacp`, `webmin` |
