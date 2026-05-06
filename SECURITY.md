# Security Policy

## Scope

This policy covers the `klim` binary and its build/release pipeline. It does **not** cover:

- Third-party CLI tools that klim detects or upgrades
- The native package managers klim invokes (brew, winget, apt, etc.)

## Reporting a Vulnerability

If you discover a security vulnerability in klim, please report it responsibly.

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Instead, use [GitHub's private vulnerability reporting](https://github.com/nassiharel/klim/security/advisories/new) to submit your report.

You should receive an acknowledgment within 48 hours. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Supported Versions

Only the latest release is actively supported with security updates.

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |
| Older   | No        |

## Verification

All releases include:

- **Checksums** — `checksums.txt` is published with every release for integrity verification.
- **SBOM** — A Software Bill of Materials is generated for each release archive.
- **Reproducible builds** — Binaries are built with `-trimpath` and `CGO_ENABLED=0` for deterministic output.
