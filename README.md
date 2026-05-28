# parse-roll20-log

Parse [Roll20](https://roll20.net) chat-log HTML exports into structured,
machine-readable session data — timestamps, players, characters, rolls, and
the full dice formulas behind each result.

Built for tabletop game-session pipelines: feed it the HTML page Roll20 hands
you when you save a chat log, get back JSONL or TSV you can pipe into
note-taking, narrative tools, or just `grep`.

## Install

```bash
go install github.com/old-school-gamers/parse-roll20-log/cmd/parse-roll20-log@latest
```

Or from a checkout, using the bundled [Taskfile](https://taskfile.dev/):

```bash
git clone https://github.com/old-school-gamers/parse-roll20-log
cd parse-roll20-log
task install   # go install ./cmd/parse-roll20-log → $GOBIN
# or
task build     # build ./parse-roll20-log in the working tree
```

A single static binary with no runtime dependencies.

## Saving a chat log from Roll20

`parse-roll20-log` reads the HTML page Roll20 produces when you save the
chat archive — there's no API call, just a file on disk.

1. Open your campaign in Roll20.
2. Click the gear icon at the bottom of the chat panel and choose
   **Show Archive** (or navigate to
   `https://app.roll20.net/campaigns/chatarchive/<campaign-id>`).
3. Scroll all the way to the bottom of the archive so every message renders.
4. In your browser: **File → Save Page As… → "Webpage, Complete"**.
5. The browser saves a `Chat Log for <Campaign>.html` plus a
   `Chat Log for <Campaign>_files/` directory of inline images. Only the
   HTML file is needed; the `_files/` directory can be deleted.

The repo's [`testdata/`](testdata/) directory is the conventional place to
drop a saved log for ad-hoc runs — see [testdata/README.md](testdata/README.md)
for a privacy note before committing one to a public fork.

## Usage

```bash
# Parse a chat log to JSONL (one record per message)
parse-roll20-log parse "Chat Log for Campaign.html"

# Filter to a single session date
parse-roll20-log parse --session 2026-03-17 chat.html

# Tab-separated columns instead of JSONL
parse-roll20-log parse --format tsv chat.html

# List the distinct session dates in an export
parse-roll20-log sessions chat.html

# Per-player and per-type counts (plus crit / fumble totals)
parse-roll20-log stats chat.html
```

## Output

Default format is JSONL — one self-contained JSON record per message:

```json
{
  "id": "-OSRfTjklc5ljxAzUvrS",
  "timestamp": "2025-06-10T20:49:00",
  "type": "general",
  "player": "alice",
  "character": "Tordek",
  "roll_name": "Strength (0)",
  "results": [
    {"value": "8", "formula": "Rolling 1d20+0 = (8)+0", "crit": "none"}
  ],
  "text": ""
}
```

Fields are lossless — every roll's dice expression and per-die breakdown is
preserved in `formula`, the message's Roll20 ID is preserved in `id`, and
unstructured messages keep their raw text in `text`. `crit` is one of
`"none"`, `"crit"` (Roll20's `fullcrit` marker), or `"fumble"` (`fullfail`).

`--format tsv` outputs a header row followed by tab-separated columns:
`timestamp`, `player`, `character`, `roll_name`, `results`, `text`. Multiple
results in one message join with `; ` in the `results` column.

## How it works

Uses [`golang.org/x/net/html`](https://pkg.go.dev/golang.org/x/net/html) to
parse the chat log as a real HTML tree, then walks the document looking for
`<div class="message …">` nodes and the structured spans/divs Roll20 emits
inside them (`sheet-char-name`, `sheet-roll-name`, `inlinerollresult`, …).

No regex pattern matching on the raw HTML. Roll20 exports often contain
trailing JavaScript and template content after the last real message; regex
approaches commonly swallow that bleed into a final malformed record. A tree
parser stops at the last actual message div.

## License

[Apache License 2.0](LICENSE) — Copyright 2026 Matthew Hunter.
