#!/bin/sh
# ==============================================================================
#  _  ______   ___ _____ _____ _  __   ____   ____ ____  ___ ____ _____ 
# | |/ /  _ \ / _ \_   _| ____| |/ /  / ___| / ___|  _ \|_ _|  _ \_   _|
# | ' /| |_) | | | || | |  _| | ' /   \___ \| |   | |_) || || |_) || |  
# | . \|  _ <| |_| || | | |___| . \    ___) | |___|  _ < | ||  __/ | |  
# |_|\_\_| \_\\___/ |_| |_____|_|\_\  |____/ \____|_| \_\___|_|    |_|  
#
# KROTEK SCRIPT HUB — PRODUCTION AUTOMATION & BOOTSTRAP LOADER
# ------------------------------------------------------------------------------
# Service:         vsftpd Secure FTP Daemon
# Category:        Media Streaming & Storage (vsftpd)
# Default Ports:   21 (Command), 40000-40100 (Passive Ports)
# Runtime Tech:    Very Secure FTP Daemon with TLS/SSL
# Global CLI Tool: sudo vsftpd-manager
# Supported OS:    Debian, Ubuntu, RHEL, AlmaLinux, Rocky Linux, Alpine Linux
# Architectures:   x86_64 (amd64), aarch64 (arm64)
# Service ID:      vsftpd
#
# OVERVIEW:
# Fast and secure FTP service with chroot jail isolation, explicit TLS encryption, and virtual user support.
#
# This bootstrap script securely downloads, cryptographically validates (SHA-256),
# and launches the compiled Go management runtime. It provides both an interactive
# Terminal User Interface (TUI) and headless non-interactive automation flags.
#
# USAGE:
#   Interactive TUI Mode (Terminal Dashboard):
#     curl -fsSL https://kroteknet.com/i/vsftpd | sudo sh
#     wget -qO- https://kroteknet.com/i/vsftpd | sudo sh
#
#   Unattended / Non-Interactive Automation:
#     curl -fsSL https://kroteknet.com/i/vsftpd | sudo sh -s -- --install -y
#
#   Permanent System CLI Tool (Installed automatically):
#     sudo vsftpd-manager
#
# AVAILABLE FLAGS:
#   --non-interactive (-y, --yes, -s, --silent)                 : Enables fully unattended mode. Skips interactive TUI menus and proceeds immediately using supplied flags or defaults.
#   --mode (-m) <native|docker> : Selects the installation stack architecture: direct host system daemon (native) or container stack (docker).
#   --install (-i)                 : Forces clean installation wizard setup even if an existing service or container is detected on the system.
#   --force (-f)                : Overwrites existing files, purges conflicting configurations and forces fresh setup.
#   --reinstall                 : Bypasses existing installation conflict prompt and executes guided setup wizard.
#   --uninstall                 : Executes clean uninstallation, stops background daemons, cleans up systemd units and optionally purges data.
#
# SECURITY & INTEGRITY SPECIFICATIONS:
#   - Fail-Closed Verification: Strict SHA-256 hash comparison against server manifest.
#   - Sandbox Isolation: Runs in a private temporary directory (mktemp -d, mode 0700).
#   - POSIX Signal Trap: Immediate automatic cleanup of temporary files on exit or abort.
#   - Root Enforcement: Prevents partial execution by unprivileged accounts.
# ==============================================================================


set -eu

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export PATH

TMP_DIR=""
STAGE_BIN=""

cleanup() {
    if [ -n "${STAGE_BIN:-}" ] && [ -e "$STAGE_BIN" ]; then
        rm -f "$STAGE_BIN" 2>/dev/null || true
        STAGE_BIN=""
    fi
    if [ -n "${TMP_DIR:-}" ] && [ -d "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR" 2>/dev/null || true
        TMP_DIR=""
    fi
}

error_exit() {
    printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2
    cleanup
    exit 1
}

is_local_http_url() {
    case "$1" in
        http://127.0.0.1|http://127.0.0.1/*|http://127.0.0.1:*|http://localhost|http://localhost/*|http://localhost:*|http://"[::1]"|http://"[::1]"/*|http://"[::1]":*)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

validate_download_url() {
    _url="$1"
    _nl='
'
    case "$_url" in
        ""|*"$_nl"*|*" "*|*"	"*|*@*|*\$*|*\`*|*\'*|*"\""*|*\;*|*\|*|*\&*|*\<*|*\>*)
            return 1
            ;;
    esac
    case "$_url" in
        https://*)
            _host_part="${_url#https://}"
            case "$_host_part" in
                ""|/*|.*) return 1 ;;
            esac
            return 0
            ;;
        http://*)
            if is_local_http_url "$_url"; then
                return 0
            fi
            return 1
            ;;
        *)
            return 1
            ;;
    esac
}

try_download() {
    _target_url="$1"
    _dest_file="$2"

    if ! validate_download_url "$_target_url"; then
        printf "\033[0;31m[ERROR]\033[0m Refusing insecure or malformed URL: %s\n" "$_target_url" >&2
        return 1
    fi

    case "$_target_url" in
        https://*)
            if command -v curl >/dev/null 2>&1; then
                curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 5 -o "$_dest_file" "$_target_url" >/dev/null 2>&1
            elif command -v wget >/dev/null 2>&1; then
                if wget --help 2>&1 | grep -q -- '--https-only'; then
                    wget -q --https-only --secure-protocol=TLSv1_2 -T 5 -O "$_dest_file" "$_target_url" >/dev/null 2>&1
                else
                    # BusyBox wget (e.g. Alpine) lacks --https-only / --secure-protocol; URL scheme https:// is validated above
                    wget -q -T 5 -O "$_dest_file" "$_target_url" >/dev/null 2>&1
                fi
            else
                return 1
            fi
            ;;
        http://*)
            # Explicit local test endpoints only (127.0.0.1 / localhost / [::1])
            if command -v curl >/dev/null 2>&1; then
                curl -fsSL --proto '=http,https' --proto-redir '=https' --connect-timeout 5 -o "$_dest_file" "$_target_url" >/dev/null 2>&1
            elif command -v wget >/dev/null 2>&1; then
                wget -q -T 5 -O "$_dest_file" "$_target_url" >/dev/null 2>&1
            else
                return 1
            fi
            ;;
        *)
            return 1
            ;;
    esac
}

is_valid_sha256() {
    _h="$1"
    case "$_h" in
        ""|*[!0-9a-fA-F]*) return 1 ;;
    esac
    [ "${#_h}" -eq 64 ]
}

main() {
    # ─── Step 1: Root Privilege Verification ──────────────────────────────────
    # Superuser privileges are required to manage system daemons, configure firewall rules,
    # install system packages, and bind to standard service ports.
    if [ "$(id -u)" -ne 0 ]; then
        printf "\033[0;31m[INFO]\033[0m This script must be run as root. Run as root ('su -' on Alpine) or with 'sudo'.\n" >&2
        exit 1
    fi

    # ─── Step 2: CPU Architecture Detection & Integrity Manifest ─────────────
    # Map detected host architecture to the appropriate static 64-bit ELF binary build
    # and cryptographic SHA-256 integrity hash.
    EXPECTED_HASH_AMD64="7b6f49a1364576a190c28f14f743a79f5a2fd26fd5b1593d39fe39156e6fdc51"
    EXPECTED_HASH_ARM64="600c09f3f2619723f3551d874864f5de05121a7a340026fabee3de807328043e"
    STATIC_HASH=""

    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)
            BINARY="krotek-vsftpd-linux-amd64"
            STATIC_HASH="$EXPECTED_HASH_AMD64"
            ;;
        aarch64|arm64)
            BINARY="krotek-vsftpd-linux-arm64"
            STATIC_HASH="$EXPECTED_HASH_ARM64"
            ;;
        *)
            printf "\033[0;31m[ERROR]\033[0m Unsupported architecture: %s\n" "$ARCH" >&2
            exit 1
            ;;
    esac

    # ─── Step 3: Secure Isolated Workspace & Signal Cleanup Trap ────────────
    # Create a sandboxed temporary directory restricted to root (0700).
    # Guarantee automatic removal upon successful exit, error, or manual cancellation.
    trap cleanup EXIT INT TERM HUP QUIT
    TMP_DIR=$(mktemp -d)
    chmod 700 "$TMP_DIR"

    BASE_URL="https://kroteknet.com/binaries"

    # Fallback if placeholder was not replaced (raw template local dev only)
    case "$BASE_URL" in
        *__"SERVER_HOST"__*) BASE_URL="http://127.0.0.1:3000/binaries" ;;
    esac

    DOWNLOAD_URL="${BASE_URL}/${BINARY}"
    AUTHORITATIVE_URL="${DOWNLOAD_URL}"
    printf "\033[38;5;117m[INFO]\033[0m Detected architecture: \033[1m%s\033[0m\n" "$ARCH"

    # Determine if non-interactive mode was requested by the user
    IS_NON_INTERACTIVE=false
    for _arg in "$@"; do
        case "$_arg" in
            -y|--yes|--non-interactive|-s|--silent) IS_NON_INTERACTIVE=true ;;
        esac
    done

    SUCCESS=false

    # Derive service subdirectory from binary name (e.g. krotek-nginx-linux-amd64 -> nginx)
    SVC_SUBDIR="${BINARY#krotek-}"
    SVC_SUBDIR="${SVC_SUBDIR%-linux-*}"
    SVC_URL="${BASE_URL}/${SVC_SUBDIR}/${BINARY}"

    printf "\033[38;5;117m[INFO]\033[0m Fetching pre-compiled Go engine: \033[1m%s\033[0m...\n" "$SVC_URL"

    # ─── Step 4: Download Pre-Compiled Go Binary ─────────────────────────────
    # Try service subfolder URL first (clean directory structure), fallback to flat URL
    if try_download "$SVC_URL" "$TMP_DIR/krotek-installer"; then
        SUCCESS=true
        DOWNLOAD_URL="$SVC_URL"
        AUTHORITATIVE_URL="$SVC_URL"
    elif try_download "$DOWNLOAD_URL" "$TMP_DIR/krotek-installer"; then
        SUCCESS=true
    fi

    # Fallback 1: Try SSH client IP if running over SSH (only when KROTEK_DEV is explicitly set)
    if [ "$SUCCESS" = false ] && [ -n "${KROTEK_DEV:-}" ] && [ -n "${SSH_CLIENT:-}" ]; then
        SSH_HOST_IP=$(printf '%s' "$SSH_CLIENT" | awk '{print $1}')
        if [ -n "$SSH_HOST_IP" ]; then
            case "$SSH_HOST_IP" in
                127.0.0.1) SSH_URL="http://127.0.0.1:3000/binaries/${BINARY}" ;;
                ::1)       SSH_URL="http://[::1]:3000/binaries/${BINARY}" ;;
                *:*)       SSH_URL="https://[${SSH_HOST_IP}]/binaries/${BINARY}" ;;
                *)         SSH_URL="https://${SSH_HOST_IP}/binaries/${BINARY}" ;;
            esac
            if try_download "$SSH_URL" "$TMP_DIR/krotek-installer"; then
                SUCCESS=true
                DOWNLOAD_URL="$SSH_URL"
            fi
        fi
    fi

    # Fallback 2: Network Gateway IP (only when KROTEK_DEV is explicitly set)
    if [ "$SUCCESS" = false ] && [ -n "${KROTEK_DEV:-}" ]; then
        GW_IP=$(ip route show default 2>/dev/null | awk '/default/ {print $3; exit}' || true)
        if [ -n "$GW_IP" ]; then
            case "$GW_IP" in
                127.0.0.1) GW_URL="http://127.0.0.1:3000/binaries/${BINARY}" ;;
                ::1)       GW_URL="http://[::1]:3000/binaries/${BINARY}" ;;
                *:*)       GW_URL="https://[${GW_IP}]/binaries/${BINARY}" ;;
                *)         GW_URL="https://${GW_IP}/binaries/${BINARY}" ;;
            esac
            if try_download "$GW_URL" "$TMP_DIR/krotek-installer"; then
                SUCCESS=true
                DOWNLOAD_URL="$GW_URL"
            fi
        fi
    fi

    # Fallback 4: Interactive Host prompt attached to TTY (only in interactive mode)
    if [ "$SUCCESS" = false ] && [ "$IS_NON_INTERACTIVE" = false ]; then
        printf "\033[0;31m[ERROR]\033[0m Could not connect to default server URL (%s).\n" "$DOWNLOAD_URL"
        printf "\033[38;5;117m[INFO]\033[0m Please enter your HTTPS host server domain or local test address (e.g. kroteknet.com):\n"

        KROTEK_PROMPT_PTR=">"
        if [ "${KROTEK_SYMBOLS:-}" = "fancy" ] || { [ "${KROTEK_SYMBOLS:-}" != "ascii" ] && [ -n "${WT_SESSION:-}${COLORTERM:-}${TERM_PROGRAM:-}" ] && case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in *[uU][tT][fF]-8*|*[uU][tT][fF]8*) true;; *) false;; esac; }; then
            KROTEK_PROMPT_PTR="▸"
        fi
        USER_HOST=""
        if [ -t 0 ]; then
            printf "  %s Server Address: " "$KROTEK_PROMPT_PTR"
            read -r USER_HOST || USER_HOST=""
        elif [ -r /dev/tty ] && [ -c /dev/tty ] && (exec </dev/tty) 2>/dev/null; then
            printf "  %s Server Address: " "$KROTEK_PROMPT_PTR" >/dev/tty
            read -r USER_HOST </dev/tty || USER_HOST=""
        fi
        USER_HOST=$(printf '%s' "$USER_HOST" | tr -cd 'a-zA-Z0-9.:/\[\]-')

        if [ -n "$USER_HOST" ]; then
            case "$USER_HOST" in
                https://*|http://*) ;;
                127.0.0.1|127.0.0.1/*|127.0.0.1:*|localhost|localhost/*|localhost:*|"[::1]"|"[::1]"/*|"[::1]":*)
                    USER_HOST="http://${USER_HOST}"
                    ;;
                *)
                    USER_HOST="https://${USER_HOST}"
                    ;;
            esac
            USER_HOST="${USER_HOST%/}"
            DOWNLOAD_URL="${USER_HOST}/binaries/${BINARY}"
            printf "\033[38;5;117m[INFO]\033[0m Retrying download from: \033[1m%s\033[0m...\n" "$DOWNLOAD_URL"
            if try_download "$DOWNLOAD_URL" "$TMP_DIR/krotek-installer"; then
                SUCCESS=true
            fi
        fi
    fi

    if [ "$SUCCESS" = false ]; then
        error_exit "Unable to download Go binary installer from $DOWNLOAD_URL. Please verify network/TLS connectivity and ensure host server is running."
    fi

    # ─── Step 5: Strict Cryptographic SHA-256 Integrity Verification (Fail-Closed) ─
    # Verify cryptographic checksum against the immutable manifest or embedded hash.
    # If the hash is unresolved, invalid, uncheckable, or mismatches, abort immediately.
    printf "\033[38;5;117m[INFO]\033[0m Verifying SHA256 cryptographic checksum...\n"

    EXPECTED_HASH=""
    case "$STATIC_HASH" in
        ""|*__SHA256_*)
            MANIFEST_URL="${AUTHORITATIVE_URL}.sha256"
            if ! try_download "$MANIFEST_URL" "$TMP_DIR/krotek-installer.sha256"; then
                rm -f "$TMP_DIR/krotek-installer"
                error_exit "Unable to fetch SHA256 checksum manifest from authoritative URL: $MANIFEST_URL"
            fi
            EXPECTED_HASH=$(awk 'NR==1 {gsub(/\r/, ""); print $1}' "$TMP_DIR/krotek-installer.sha256")
            ;;
        *)
            EXPECTED_HASH="$STATIC_HASH"
            ;;
    esac

    case "$EXPECTED_HASH" in
        ""|*__SHA256_*)
            rm -f "$TMP_DIR/krotek-installer"
            error_exit "SHA256 checksum is missing or contains an unresolved placeholder."
            ;;
    esac

    if ! is_valid_sha256 "$EXPECTED_HASH"; then
        rm -f "$TMP_DIR/krotek-installer"
        error_exit "SHA256 checksum is invalid (must be exactly 64 hexadecimal characters)."
    fi
    EXPECTED_HASH=$(printf '%s' "$EXPECTED_HASH" | tr '[:upper:]' '[:lower:]')

    CALC_HASH=""
    if command -v sha256sum >/dev/null 2>&1; then
        CALC_HASH=$(sha256sum "$TMP_DIR/krotek-installer" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        CALC_HASH=$(shasum -a 256 "$TMP_DIR/krotek-installer" | awk '{print $1}')
    elif command -v openssl >/dev/null 2>&1; then
        CALC_HASH=$(openssl dgst -sha256 "$TMP_DIR/krotek-installer" | awk '{print $NF}')
    else
        rm -f "$TMP_DIR/krotek-installer"
        error_exit "No SHA256 verification tool found ('sha256sum', 'shasum', or 'openssl'). Cannot verify cryptographic integrity; aborting."
    fi

    if ! is_valid_sha256 "$CALC_HASH"; then
        rm -f "$TMP_DIR/krotek-installer"
        error_exit "Failed to calculate valid SHA256 digest of downloaded binary."
    fi
    CALC_HASH=$(printf '%s' "$CALC_HASH" | tr '[:upper:]' '[:lower:]')

    if [ "$CALC_HASH" != "$EXPECTED_HASH" ]; then
        printf "\033[0;31m[CRITICAL SECURITY ERROR]\033[0m SHA256 Checksum MISMATCH!\n" >&2
        printf "  Expected   : %s\n" "$EXPECTED_HASH" >&2
        printf "  Calculated : %s\n" "$CALC_HASH" >&2
        rm -f "$TMP_DIR/krotek-installer"
        error_exit "Aborting immediately. The downloaded binary has been tampered with or corrupted."
    fi

    printf "\033[38;5;114m[SECURITY OK]\033[0m SHA256 integrity verified successfully: %s\n" "$CALC_HASH"
    chmod 0700 "$TMP_DIR/krotek-installer"

    # Secure interactive inspection window: allow human operator to visually verify the cryptographic fingerprint.
    # Automatically skipped in unattended / non-interactive pipelines (CI/CD, Docker, piped stdout).
    if [ "${IS_NON_INTERACTIVE:-false}" = false ] && [ "${KROTEK_NON_INTERACTIVE:-0}" != "1" ] && [ "${CI:-}" != "true" ] && [ "${DEBIAN_FRONTEND:-}" != "noninteractive" ] && [ -t 1 ]; then
        printf "\033[38;5;117m[INFO]\033[0m Cryptographic verification confirmed. Initializing console in 2s...\n"
        sleep 2 2>/dev/null || true
    fi

    # ─── Step 6: Install Global Management CLI Commands (Fail-Closed) ───────
    # Atomically install the verified root-owned binary (0755) into /usr/local/bin.
    mkdir -p /usr/local/bin || error_exit "Failed to ensure /usr/local/bin exists."
    STAGE_BIN=$(mktemp "/usr/local/bin/.krotek-vsftpd.XXXXXX") || error_exit "Failed to allocate temporary staging file in /usr/local/bin."
    cp "$TMP_DIR/krotek-installer" "$STAGE_BIN" || error_exit "Failed to copy verified binary to /usr/local/bin."
    chown 0:0 "$STAGE_BIN" 2>/dev/null || true
    chmod 0755 "$STAGE_BIN" || error_exit "Failed to set mode 0755 on $STAGE_BIN."
    mv -f "$STAGE_BIN" "/usr/local/bin/vsftpd-manager" || error_exit "Failed to atomically install /usr/local/bin/vsftpd-manager."
    STAGE_BIN=""

    rm -f "/usr/local/bin/vsftpd-installer"
    ln -sf "/usr/local/bin/vsftpd-manager" "/usr/local/bin/vsftpd-installer" || error_exit "Failed to create symlink /usr/local/bin/vsftpd-installer."

    # ─── Step 7: Hand Over Control to Go Runtime Engine ───────────────────────
    # Safe check if /dev/tty can be used when invoked via 'curl | sh' without failing in CI/Docker (ENXIO)
    USE_TTY=false
    if [ "$IS_NON_INTERACTIVE" = false ]; then
        if [ -t 0 ]; then
            USE_TTY=false
        elif [ -r /dev/tty ] && [ -c /dev/tty ] && (exec </dev/tty) 2>/dev/null; then
            USE_TTY=true
        fi
    fi

    # Detect if an explicit action was supplied (avoid forcing --install if user passed other commands/flags)
    HAS_ACTION=false
    for _arg in "$@"; do
        case "$_arg" in
            --install|-i|install|--reinstall|reinstall|--force|-f|force|--uninstall|uninstall|--status|status|--help|-h|help|--version|-v|version|ps|docker|containers|list|ls|doctor|firewall|ssh) HAS_ACTION=true ;;
        esac
    done

    TARGET_BIN="/usr/local/bin/vsftpd-manager"
    if [ ! -x "$TARGET_BIN" ]; then
        error_exit "Installed binary $TARGET_BIN is missing or not executable."
    fi

    rc=0
    if [ "${HAS_ACTION:-false}" = true ]; then
        if [ "${USE_TTY:-false}" = true ]; then
            "$TARGET_BIN" "$@" </dev/tty || rc=$?
        else
            "$TARGET_BIN" "$@" || rc=$?
        fi
    else
        if [ "${USE_TTY:-false}" = true ]; then
            "$TARGET_BIN" --install "$@" </dev/tty || rc=$?
        else
            "$TARGET_BIN" --install "$@" || rc=$?
        fi
    fi

    cleanup
    exit "$rc"
}

main "$@"
