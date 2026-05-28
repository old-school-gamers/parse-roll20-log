# parse-roll20-log

[![CI](https://github.com/old-school-gamers/parse-roll20-log/actions/workflows/ci.yml/badge.svg)](https://github.com/old-school-gamers/parse-roll20-log/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/old-school-gamers/parse-roll20-log.svg)](https://pkg.go.dev/github.com/old-school-gamers/parse-roll20-log)
[![Go Report Card](https://goreportcard.com/badge/github.com/old-school-gamers/parse-roll20-log)](https://goreportcard.com/report/github.com/old-school-gamers/parse-roll20-log)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Parse [Roll20](https://roll20.net) chat-log HTML exports into structured,
machine-readable session data — timestamps, players, characters, rolls, and
the full dice formulas behind each result.

Built for tabletop game-session pipelines: feed it the HTML page Roll20 hands
you when you save a chat log, get back JSONL or TSV you can pipe into
note-taking, narrative tools, or just `grep`.

## Why

I had a Python script that parsed Roll20 chat logs for D&D session notes.
It worked, but the regex approach quietly mishandled trailing JavaScript
that Roll20 appends to every export — the last "message" would balloon
into ~60,000 characters of HTML/JS bleed. The pipeline workaround was an
`awk` filter that dropped the garbage line but also dropped most of the
real messages with it. The script also threw away the dice formulas Roll20
records on every roll result, which turn out to be useful.

This is a Go rewrite that uses a real HTML tree parser instead of regex
(so the tail-bleed bug is gone by construction), keeps the per-die formulas
and crit / fumble markers, emits structured JSONL or TSV, and ships as a
single static binary with no Python interpreter, no virtualenv, and nothing
to install at runtime.

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

### Shell completion

Cobra auto-generates completion scripts. For bash:

```bash
mkdir -p ~/.local/share/bash-completion/completions
parse-roll20-log completion bash > ~/.local/share/bash-completion/completions/parse-roll20-log
```

`zsh`, `fish`, and `powershell` work too — run
`parse-roll20-log completion <shell> --help` for the install path each
shell expects.

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
drop your saved log for ad-hoc runs. It's gitignored — nothing dropped
there gets tracked. A Roll20 export bundles Roll20's own jQuery and CSS
framework alongside the chat content, so it's never safe to commit to a
public repo regardless of whether the chat itself is sanitized.

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

### Output stability

The JSONL field set may **grow** across 0.x releases — new fields are
additive and won't break a consumer that ignores unknown keys. Field
**renames** and **removals** are breaking changes and bump the minor
version (in 0.x) or the major version (post-1.0). The set of supported
`type` and `crit` values may also grow across 0.x.

## Roll20 compatibility

Currently tested against **one Shadowdark campaign export** (the campaign
this tool was originally written for). Coverage for that case is good —
1,755 rollresult, 750 emote, and 3,926 general messages across 78 sessions
parse cleanly with full structured fields.

What should work for any Roll20 campaign regardless of system:

- Timestamps, players, and message types (`general`, `emote`, `rollresult`).
- Inline roll **values** and **formulas** (the `inlinerollresult` element
  is Roll20-core, not sheet-specific).
- Discord-routed `/r` outputs (the `rollresult` message type with
  `formula` + `rolled` divs).

What may come back **empty for other systems**: `character` and
`roll_name`. These are extracted from a specific set of element class
names Shadowdark's sheets use (`sheet-char-name`, `sheet-roll-name`,
`sheet-trait-name`, `sheet-feature-name`). D&D 5e, Pathfinder, Savage
Worlds, etc. each ship their own rolltemplate shapes with different field
names. If you run this against another system and find character /
roll-name extraction missing, please [open an
issue](https://github.com/old-school-gamers/parse-roll20-log/issues) with
a representative HTML snippet — adding a template is a small parser change.

The tool also runs a sanity check after parsing: if it sees many messages
but no rolls at all, it prints a warning to stderr, since that usually
means Roll20 changed an HTML class name we depend on.

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

---

Roll20 is a trademark of The Orr Group, LLC. This project is not affiliated
with or endorsed by Roll20.
