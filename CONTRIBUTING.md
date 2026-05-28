# Contributing

Thanks for thinking about contributing — bug reports, fixes, and small
features are all welcome.

## Development

[Task](https://taskfile.dev/) wraps the standard commands:

```bash
task test     # go test -race -count=1 ./...
task check    # everything CI runs: test + vet + fmt + lint + vulncheck
task build    # produce ./parse-roll20-log in the working tree
task install  # go install ./cmd/parse-roll20-log → $GOBIN
```

CI runs `task check` on every push and PR — please run it locally first.
`task fmt:fix` (which calls `gofmt -w .`) handles formatting drift.

## Test fixtures

Synthetic HTML for the unit tests lives in
[`internal/parser/testdata/`](internal/parser/testdata/) and is intentionally
tiny — add new fixtures (or extend `messages.html`) when a real export
contains a Roll20 element shape we don't yet handle. Keep fixtures
hand-crafted; don't paste in real chat content.

Real saved exports go in [`testdata/`](testdata/) at the repo root and stay
on your machine — that directory is fully gitignored and contains Roll20's
own framework HTML, so nothing real should ever be committed.

## Scope

The tool's scope is intentionally narrow: parse Roll20 chat-log HTML into
structured output. Adjacent ideas like rolltemplate-aware analytics,
multi-export merging, or D&D rules interpretation are interesting but
probably belong in separate tools that consume this one's JSONL.

Bug reports for real Roll20 element shapes we don't yet handle are
especially useful. A minimal HTML snippet reproducing the issue is worth a
lot more than a description.

## License

By submitting a contribution, you agree to license it under the same
[Apache License 2.0](LICENSE) that covers the rest of the project.
