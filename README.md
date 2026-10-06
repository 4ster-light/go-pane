# go-pane

[![CI](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml/badge.svg)](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/4ster-light/go-pane)](go.mod)
[![License: MIT](https://img.shields.io/github/license/4ster-light/go-pane)](LICENSE)
[![KDE Plasma 6](https://img.shields.io/badge/KDE%20Plasma-6-blue)](https://kde.org/plasma-desktop/)
[![GitHub last commit](https://img.shields.io/github/last-commit/4ster-light/go-pane)](https://github.com/4ster-light/go-pane/commits/main)

A native KDE Plasma 6 widget that shows live **OpenCode Go** subscription usage
(rolling / weekly / monthly) with reset countdowns and derived pacing metrics.

The logic lives in a small Go daemon (`go-pane`); the applet is a thin QML
plasmoid that renders what the daemon serves over loopback HTTP.

```
OpenCode Go API ──> go-pane daemon (HTTP) ──> Plasma applet (QML)

   /zen/go/v1/usage     (Go)         127.0.0.1:17873
```

## Features

- Rolling, weekly and monthly usage, matching the OpenCode dashboard.
- **Available % left** and **reset countdown** (`19d 5h`, `4h 59m`, …).
- Derived metrics: pace ratio, projected usage at reset, exhaustion ETA and a
  safe `%/h` budget.
- Compact panel representation + detailed popup, configurable thresholds.
- Learns the real window lengths from history (rolling ≈ 5h, weekly 7d, monthly
  is a signup anniversary).
- Keeps the API key out of QML/KConfig; only the daemon sees it.

## Requirements

- KDE Plasma 6 (`plasma5support`, `kpackagetool6`)
- Go 1.23+ (to build)
- An OpenCode **API key** (`oc_sk_…`). The OAuth token used by the opencode CLI
  is *not* accepted by the usage endpoint.

## Build & install

```sh
make build          # -> bin/go-pane
make test
make install        # binary -> ~/.local/bin, unit -> ~/.config/systemd/user, plasmoid
systemctl --user daemon-reload
systemctl --user enable --now go-pane.service
```

Then right-click your panel → *Add Widgets…* → **OpenCode Go Usage**.

For iterating on the plasmoid:

```sh
make dev            # kpackagetool6 -u + plasmawindowed
```

## API key discovery

`go-pane` looks for the key in this order (first match wins):

1. `--api-key-file PATH`
2. `OPENCODE_API_KEY` or `OPENCODE_GO_API_KEY`
3. `~/.config/go-pane/config.json` → `apiKey` / `apiKeyFile`
4. `~/.config/go-pane/api_key` (use `chmod 600`)
5. `~/.pi/agent/auth.json` → `opencode-go.key`

Check everything with:

```sh
go-pane doctor
```

## CLI

```sh
go-pane serve   [--addr 127.0.0.1:17873] [--interval 60s] [--base-url URL] [--api-key-file PATH]
go-pane once    [--json]        # one-shot, text or JSON
go-pane json                    # one-shot JSON (daemon not required)
go-pane doctor                  # verify key + connectivity
go-pane version
```

`once --json` can be wired to the `plasma5support` "executable" data engine if
you prefer not to run the daemon.

## HTTP API (loopback only)

- `GET /healthz`
- `GET /v1/usage` — the full computed snapshot
- `GET /v1/history?window=monthly&limit=288` — raw samples for charts

## Configuration

Edit via the widget's settings, or `~/.config/go-pane/config.json`:

```json
{
  "addr": "127.0.0.1:17873",
  "interval": "60s",
  "baseURL": "https://opencode.ai/zen/go/v1",
  "apiKeyFile": "~/.config/go-pane/api_key"
}
```

## Notes

- The usage endpoint (`GET /zen/go/v1/usage`) is undocumented and may change;
  the base URL is configurable.
- Window lengths are inferred. The daemon refines them from reset timestamps
  stored in `~/.local/state/go-pane/`.

## Development

```sh
make build      # build bin/go-pane
make test       # go test ./...
make check      # gofmt check + go vet + tests + golangci-lint
make lint       # golangci-lint run ./...
make dev        # upgrade the plasmoid and open it with plasmawindowed
```

CI (`.github/workflows/ci.yml`) runs formatting, `go vet`, race tests with
coverage, and golangci-lint on every push and pull request.

## License

MIT
