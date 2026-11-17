# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in any Go manager, bootstrap `install.sh` script, or binary distribution pipeline, please **do not** open a public GitHub Issue.

Report it privately via:
- **Email:** [support@kroteknet.com](mailto:support@kroteknet.com)
- **RFC 9116 Contact:** [https://kroteknet.com/.well-known/security.txt](https://kroteknet.com/.well-known/security.txt)

Please include:
- The affected service/script path and version/architecture.
- Steps to reproduce or a minimal proof of concept.
- Potential impact (e.g., privilege escalation, command injection, TLS/checksum bypass).

## Integrity & Verification

- Every pre-compiled Linux binary in `binaries/` is accompanied by a `.sha256` file and listed in `binaries/checksums.txt`.
- Bootstrap scripts enforce fail-closed SHA-256 verification and refuse execution on checksum mismatch or non-HTTPS download URLs (except explicit local loopback testing).
- Full legal terms and disclaimers are published at [https://kroteknet.com/terms](https://kroteknet.com/terms).
