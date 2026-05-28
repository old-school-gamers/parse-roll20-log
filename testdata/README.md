# testdata/

Drop real Roll20 chat-log HTML exports here to run the tool against actual
data. Nothing in this directory is required for the unit tests — those use
small synthetic fixtures in `internal/parser/testdata/`. This directory is
just a conventional landing spot for ad-hoc samples and smoke tests.

## How to obtain a Roll20 chat log

1. Open your Roll20 campaign in a browser.
2. Click the gear icon at the bottom of the chat panel, then "Show Archive"
   (or navigate directly to `https://app.roll20.net/campaigns/chatarchive/<campaign-id>`).
3. Scroll to the bottom of the archive so the page loads every message.
4. Use your browser's **File → Save Page As… → "Webpage, Complete"**.
5. Browsers save the file as something like
   `Chat Log for <Campaign Name>.html`, alongside a `Chat Log for <Campaign Name>_files/`
   directory of inline images. The HTML is the only file `parse-roll20-log`
   needs; the `_files/` directory can be deleted.

A full campaign archive is typically several megabytes. The file lives
entirely on disk after the save — no network round-trips during parsing.

## Privacy note

Roll20 chat archives contain real player handles, character names, and chat
content. If you commit a sample into a public repo, sanitize it first
(replace player and character names, redact in-character text). Files in
this directory are **not** auto-gitignored — that's a deliberate choice
because a sanitized public sample is genuinely useful, but it means you
have to be careful what you `git add`.

## Quick start

```bash
parse-roll20-log sessions "testdata/Chat Log for My Campaign.html"
parse-roll20-log parse --session 2026-03-17 "testdata/Chat Log for My Campaign.html"
parse-roll20-log stats "testdata/Chat Log for My Campaign.html"
```
