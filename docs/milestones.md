# Milestones

## M1: Config & Feed Parsing

- Define YAML config schema (feeds list, email settings, check interval)
- Parse config file
- Fetch and parse RSS/Atom feeds
- CLI entrypoint that loads config and prints parsed feed items
- Basic tests

## M2: Email Rendering

- HTML email template — clean, readable, mobile-friendly
- Render each item as: title (linked to original), date, full content from RSS `<content:encoded>` or `<description>`
- Plain-text fallback version
- Test with sample feed data

## M3: Email Sending via SMTP

- SMTP config in YAML (host, port, user, password, from, to)
- Send rendered emails — one email per item
- Test with a real SMTP server

## M4: State Tracking (Deduplication)

- Track which items have already been sent (by GUID/link)
- JSON state file alongside the binary
- On each run, only process unseen items
- Handle feed items disappearing from the feed gracefully

## M5: Run Loop & Polish

- Two modes: one-shot (`mailfeed run`) and periodic (`mailfeed daemon` with interval from config)
- Graceful shutdown on SIGINT/SIGTERM
- Logging (structured, minimal)
- Error handling: unreachable feeds, SMTP failures, malformed items
- Build as static binary (`CGO_ENABLED=0`)

## M6: Edge Cases & Hardening

- Feed autodiscovery (follow redirects, handle common URL patterns)
- Timeout/retry on feed fetches
- Content sanitization (strip unsafe HTML for email clients)
- Config validation with clear error messages
