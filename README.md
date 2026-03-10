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
| `smtp.username` | No | SMTP auth username |
| `smtp.password` | No | SMTP auth password |
| `smtp.tls` | No | TLS mode: `"implicit"`, `"starttls"`, or `""` (auto-detect based on port) |

### `check_interval` (optional)

How often to check feeds in `loop` mode. Uses Go duration syntax (`"30m"`, `"1h"`, `"2h30m"`). Required for the `loop` subcommand.

### `user_agent` (optional)

Custom User-Agent string for HTTP requests. Defaults to `"mailfeed/1.0"`.

## State File

mailfeed tracks sent items in a JSON state file (default: `state.json`). When a new feed is added, only the latest item is sent — older items are marked as already seen. State is saved after each email, so a crash mid-run won't cause duplicates on restart.

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
