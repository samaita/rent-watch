# Rent Watcher

`rent-watcher` is a Go service that periodically checks configured rental listing pages, stores the observed state in SQLite, and emits updates through Telegram when listings appear, disappear, or change price.

## Problem

Manual rental hunting is repetitive and easy to miss. Listings can appear, change, or disappear between checks, especially on marketplace-style sites.

## Goal

Run a small watcher that:

- scrapes configured rental search pages on a schedule
- stores listing history locally in SQLite
- reports availability and price-change events
- optionally exposes simple Telegram commands for status checks

## Requirements

- Go `1.26.2`
- A Chrome or Chromium binary available locally
- Optional: a Telegram bot token and allowed user IDs for notifications

## Setup

1. Copy the example environment file and fill in values:

```bash
cp .env.example .env
```

2. Prepare a seed file for the watch pages:

```bash
cp seed.example.yaml seed.yaml
```

3. The app now loads configuration in two steps:

- first from the machine environment
- then from a local `.env` in the repo root for any variables that are still unset

You can still export variables explicitly when needed:

```bash
set -a
source .env
set +a
```

## Commands

Run the watcher:

```bash
make dev
```

Print the version:

```bash
go run ./cmd/rentwatch --version
```

Run tests:

```bash
go test ./...
```

Build a local binary in the repo root:

```bash
make build
```

## Environment Variables

| Variable | Required | Description |
| --- | --- | --- |
| `SQLITE_PATH` | No | Path to the SQLite database file. Default: `rent-watcher.sqlite`. |
| `SEED_PATH` | Yes | Path to the YAML seed file containing watch pages. |
| `DEBUG` | No | Enables debug-level logging when set to `true`. Default: `false`. |
| `TELEGRAM_BOT_TOKEN` | No | Telegram bot token used for notifications and commands. |
| `TELEGRAM_ALLOWED_USER_IDS` | No | Comma-separated list of Telegram user IDs allowed to receive notifications and run commands. In direct chats with the bot, the user ID is also the chat ID used for notifications. |
| `TIMEZONE` | No | Application timezone. Default: `Asia/Jakarta`. |
| `CYCLE_INTERVAL` | No | How often a full scraping cycle runs. Default: `12h`. |
| `HEARTBEAT_INTERVAL` | No | How often heartbeat notifications are sent. Default: `1h`. |
| `SAME_SITE_MIN_DELAY` | No | Minimum delay between requests to the same site. Default: `1m`. |
| `SAME_SITE_MAX_DELAY` | No | Maximum delay between requests to the same site. Default: `5m`. |
| `CHROME_HEADLESS` | No | Run Chrome in headless mode. Default: `true`. |
| `CHROME_PATH` | No | Explicit path to the Chrome or Chromium executable. |
| `SCRAPE_TIMEOUT` | No | Timeout for a single scrape session. Default: `45s`. |

## Seed File

The app expects a YAML file referenced by `SEED_PATH`. Start from `seed.example.yaml`, which currently contains sample `olx` watch pages.

## Telegram Commands

When `TELEGRAM_BOT_TOKEN` is set, the bot supports:

- `/version` to show the current app version
- `/list` to list the tracked watches

Startup logs now show whether Telegram was disabled, whether bot authentication succeeded, and when long polling becomes active. Incoming non-command messages and unsupported commands are ignored by design.

## Security Notes

- `.env`, private key material, local databases, and common secret file patterns are ignored by `.gitignore`.
- Keep real bot tokens and Telegram user IDs in `.env`, not in committed files.
- Commit `.env.example`, not `.env`.
