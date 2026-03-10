# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

mailfeed is a personal RSS-to-email tool. It fetches RSS/Atom feeds, renders items as HTML emails, and sends them via SMTP. It tracks seen items in a JSON state file to avoid duplicates. Single static binary, no web UI.

## Commands

```bash
make build     # CGO_ENABLED=0 go build -o mailfeed ./cmd/mailfeed
make test      # go test ./...
make clean     # rm -f mailfeed

# Run a single package's tests
go test ./internal/feed/

# Run a specific test
go test ./internal/feed/ -run TestFetch
```

## Architecture

**Entry point**: `cmd/mailfeed/main.go` — CLI with `once` (single run) and `loop` (daemon) subcommands. Orchestrates: load config → fetch feeds → filter new items via state → send emails → update state.

**Four internal packages** (`internal/`):

- **config** — Parses YAML config (feeds list, SMTP/email settings, check interval). Validates all fields.
- **feed** — Fetches and parses RSS/Atom/JSON feeds via `gofeed`. `FetchAll` processes multiple feeds, skipping failures if at least one succeeds. `Item` has content fallback (content:encoded → description → synthesized) and GUID fallback (guid → link → SHA256 hash).
- **state** — JSON state file tracking seen items (feed URL + GUID → timestamp). `FilterNewItems` treats new feeds specially: only returns the latest item, marks older ones as seen. Uses atomic file writes (write .tmp, rename).
- **email** — Renders multipart MIME emails (HTML + plain text) and sends via SMTP. `SendAll` uses a single connection with incremental callback for state persistence after each send. Supports implicit TLS (465), STARTTLS (587), and auto-detection.

## Documentation

Read `docs/intro.md` for an overview of requirements and key technical descisions.
Read `docs/milestones.md` for the list of implemenation milestones.

## Working with code

 * The key priorities when working with this codebase are: 1) correctness, 2) simplicity.
 * Don't overcomplicate things! Make them as simple as possible but sufficient for correctness.
 * Write thoughtful comments that explain *why* things are done. Don't leave short comments that literally just explain *what* the code is doing.
 * After making code changes always run `make lint` and fix the issues. Then run `make fmt` to reformat the code.
