# testdata/

This directory is gitignored — drop a Roll20 chat-log HTML export here
to run the tool against real data on your machine. Nothing in this
directory will ever be tracked by git (see `.gitignore`).

The unit tests don't need anything here; they use small synthetic fixtures
in `internal/parser/testdata/`. This directory exists purely as a
conventional landing spot for your own exports.

## How to save a Roll20 chat log

See ["Saving a chat log from Roll20"](../README.md#saving-a-chat-log-from-roll20)
in the project README for the browser-save walkthrough.

## Why nothing real is committed here

A Roll20 chat-log export is a self-contained HTML page that bundles Roll20's
own jQuery, CSS, and template HTML alongside the chat content. Those bits
are Roll20's intellectual property, not ours to redistribute — so even a
chat-content-sanitized sample wouldn't be safe to commit. If you fork the
repo, keep this directory empty in git.

## Quick start

```bash
parse-roll20-log sessions "testdata/Chat Log for My Campaign.html"
parse-roll20-log parse --session 2026-03-17 "testdata/Chat Log for My Campaign.html"
parse-roll20-log stats "testdata/Chat Log for My Campaign.html"
```
