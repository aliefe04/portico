# Portico

Portico is a community-driven SSH workspace for the terminal.

It aims for the same calm, useful feel as modern terminal tools: fast, readable, and focused on trust.

## Status

Portico is early, but usable. Phase 1 is a read-only Bubble Tea interface for `~/.ssh/config`.

## What it does today

- Reads your SSH config
- Shows a searchable host list
- Displays host details for the selected entry
- Stays read-only in Phase 1

## Local development

```sh
go test ./...
go run ./cmd/portico
```

## Requirements

- Go 1.24+
- macOS or Linux

## Contributing

Contributions are welcome.

- Branch from `main` for active development.
- Use short-lived feature branches.
- Keep changes small and reviewable
- Prefer safe defaults and clear UX
- Add tests for behavior changes
- Avoid adding secrets or hidden persistence

## Roadmap

- Safe config editing with backups and diff previews
- Local SSH key management
- Guided onboarding and key verification
- Release packaging and installer

## License

This project will be released under an open-source license.
