# mailfeed

A personal RSS-to-email tool. Fetches RSS/Atom/JSON feeds, renders new items as HTML emails, and sends them via SMTP. Tracks seen items in a JSON state file to avoid duplicates.

Single static binary, no web UI, no signups. Just a config file and a binary.

## Usage

```
mailfeed <once|loop> [flags]
```

### Subcommands

- **`once`** — Fetch feeds, send new items, then exit.
- **`loop`** — Run as a daemon, checking feeds on a recurring interval (requires `check_interval` in config).

### Flags

| Flag | Default | Description |
|---|---|---|
| `-config` | `config.yaml` | Path to the YAML config file |
| `-state` | `state.json` | Path to the JSON state file |
| `-dry-run` | `false` | Fetch and print new items without sending emails |

### Examples

```bash
# One-shot run with default paths
mailfeed once

# Custom config and state paths
mailfeed once -config /etc/mailfeed/config.yaml -state /var/lib/mailfeed/state.json

# Preview new items without sending
mailfeed once -dry-run

# Run as a daemon
mailfeed loop -config config.yaml
```

## Configuration

Create a YAML config file (default: `config.yaml`):

```yaml
feeds:
  - name: "Julia Evans"
    url: "https://jvns.ca/atom.xml"
  - name: "Dan Luu"
    url: "https://danluu.com/atom.xml"

email:
  from: "mailfeed@example.com"
  to: "me@example.com"
  max_per_feed: 3
  max_per_day: 50
  smtp:
    host: "smtp.fastmail.com"
    port: 465
    username: "me@fastmail.com"
    password: "app-password-here"

check_interval: "30m"
```

### `feeds` (required)

| Field | Required | Description |
|---|---|---|
| `name` | No | Display name for the feed |
| `url` | Yes | URL of the RSS, Atom, or JSON feed |

### `email` (required)

| Field | Required | Description |
|---|---|---|
| `from` | Yes | Sender email address |
| `to` | Yes | Recipient email address |
| `smtp.host` | Yes | SMTP server hostname |
| `smtp.port` | No | SMTP port (465 for implicit TLS, 587 for STARTTLS) |
| `smtp.username` | No | SMTP auth username (can also be set via `MAILFEED_SMTP_USER` env var) |
| `smtp.password` | No | SMTP auth password (can also be set via `MAILFEED_SMTP_PASSWORD` env var) |
| `smtp.tls` | No | TLS mode: `"implicit"`, `"starttls"`, or `""` (auto-detect based on port) |
| `max_per_feed` | No | Max emails to send per feed per run (0 = unlimited) |
| `max_per_day` | No | Max emails to send total per day across all runs (0 = unlimited) |

### Environment Variables

| Variable | Description |
|---|---|
| `MAILFEED_SMTP_USER` | SMTP username. Overrides `smtp.username` from the config file. |
| `MAILFEED_SMTP_PASSWORD` | SMTP password. Overrides `smtp.password` from the config file. |

### Digest Mode

High-volume feeds can be bundled into a single daily digest email instead of sending one email per item. Mark a feed with `digest: true` and set a `digest_time` (globally or per-feed):

```yaml
digest_time: "08:00"        # default time for all digest feeds
timezone: "Europe/Berlin"   # timezone for digest scheduling (default: UTC)

feeds:
  - name: "High-volume Blog"
    url: "https://example.com/feed.xml"
    digest: true             # bundles items, sends at 08:00 Berlin time
  - name: "News"
    url: "https://example.com/news.xml"
    digest: true
    digest_time: "18:00"     # per-feed override
  - name: "Alerts"
    url: "https://example.com/alerts.xml"
    # no digest — sends immediately as before
```

New items from digest feeds are accumulated in the state file. When `mailfeed once` runs after the scheduled digest time, all accumulated items are sent as a single email. Digests are capped at 50 items per email; overflow items carry over to the next cycle.

| Field | Required | Description |
|---|---|---|
| `digest_time` | No (global) | Default send time for digest feeds, in `HH:MM` format |
| `timezone` | No | Timezone for digest scheduling. Defaults to `"UTC"` |
| `feeds[].digest` | No | Set to `true` to enable digest mode for this feed |
| `feeds[].digest_time` | No | Per-feed override for digest send time |

`max_per_feed` does not apply to digest feeds. `max_per_day` counts each digest email as 1 send.

### `check_interval` (optional)

How often to check feeds in `loop` mode. Uses Go duration syntax (`"30m"`, `"1h"`, `"2h30m"`). Required for the `loop` subcommand.

### `user_agent` (optional)

Custom User-Agent string for HTTP requests. Defaults to `"mailfeed/1.0"`.

## State File

mailfeed tracks sent items in a JSON state file (default: `state.json`). When a new immediate feed is added, only the latest item is sent — older items are marked as already seen. New digest feeds are handled differently: all items are accumulated into the first digest. State is saved after each email, so a crash mid-run won't cause duplicates on restart. Pending digest items are also stored in the state file until their scheduled send time.

## Docker

```bash
docker build -t mailfeed .
docker run -v /path/to/config.yaml:/config.yaml \
           -v /path/to/state.json:/state.json \
           mailfeed loop -config /config.yaml -state /state.json
```

## Building

```bash
make build   # produces ./mailfeed (static binary, no CGO)
make test    # run all tests
make clean   # remove built binary
```
