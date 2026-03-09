# mailfeed

A personal RSS-to-email tool. It reads a list of RSS/Atom feeds from a YAML config, fetches new items, and sends each one as a clean HTML email with a link to the original and the full article content.

Inspired by [Blogtrottr](https://blogtrottr.com/), but stripped down to the essentials: no users, no signups, no web UI. Just a static binary and a config file.

## Requirements

- Single static binary, no runtime dependencies
- YAML config file with feeds list, SMTP settings, and check interval
- Supports both RSS and Atom feeds
- Sends one email per new feed item
- Renders clean, readable HTML emails with full article content
- Tracks already-sent items to avoid duplicates (JSON state file)
- When adding a new feed only sends one latest entry
- Two run modes: one-shot and periodic daemon
- No CGO — builds with `CGO_ENABLED=0` for easy deployment

## Key Technical Decisions

**Go with minimal dependencies.** Only two direct dependencies: `gopkg.in/yaml.v3` for config parsing and `github.com/mmcdole/gofeed` for RSS/Atom parsing. Everything else (SMTP, HTTP, HTML templating) uses the standard library.

**gofeed for feed parsing.** Writing a correct RSS+Atom parser by hand is tedious — date formats alone are a nightmare. gofeed handles RSS 0.9x, 1.0, 2.0, Atom, and JSON Feed, normalizing them into a single struct. Pure Go, no CGO.

**JSON for state, not SQLite.** SQLite would require CGO (or a slower pure-Go reimplementation). A JSON file mapping GUIDs to sent timestamps is sufficient for a personal tool with a few dozen feeds.

**Own `Item` struct instead of using `gofeed.Item` directly.** Decouples the domain model from the library. The internal `Item` has exactly the fields needed for email rendering and deduplication, and handles content/GUID fallback logic in one place.

**`flag` package, not cobra.** The CLI has one flag (`-config`) and will have two subcommands (`run`, `daemon`). The standard library is enough.

## Config Example

```yaml
feeds:
  - name: "Julia Evans"
    url: "https://jvns.ca/atom.xml"
  - name: "Dan Luu"
    url: "https://danluu.com/atom.xml"

email:
  from: "mailfeed@example.com"
  to: "me@example.com"
  smtp:
    host: "smtp.fastmail.com"
    port: 465
    username: "me@fastmail.com"
    password: "app-password-here"

check_interval: "30m"
```
