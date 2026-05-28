# Security Policy

## Reporting a Vulnerability

If you discover a security issue in `parse-roll20-log`, please report it by
opening a [private security advisory](https://github.com/old-school-gamers/parse-roll20-log/security/advisories/new)
on GitHub rather than filing a public issue.

This tool parses untrusted HTML files (Roll20 chat exports). Bugs to watch for:
unbounded memory use on malformed input, panics on deeply nested or truncated
HTML, and any path that lets crafted attributes inject content into the output
format (e.g. JSONL records with embedded newlines).

## Supported Versions

The latest tagged release is supported. Older releases receive fixes only at
the maintainer's discretion.
