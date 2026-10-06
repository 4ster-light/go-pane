# go-pane

[![CI](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml/badge.svg)](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/4ster-light/go-pane)](https://github.com/4ster-light/go-pane/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/4ster-light/go-pane)](go.mod)
[![License: MIT](https://img.shields.io/github/license/4ster-light/go-pane)](LICENSE)
[![KDE Plasma 6](https://img.shields.io/badge/KDE%20Plasma-6-blue)](https://kde.org/plasma-desktop/)
[![GitHub last commit](https://img.shields.io/github/last-commit/4ster-light/go-pane)](https://github.com/4ster-light/go-pane/commits/main)

> [!NOTE]
> This project is primarily for personal use, hence the specific target
> environment. However, codebase quality is also a main goal and it should work
> in any compatible environment (Plasma 6 KDE) and contributions of any kind
> (issues, feature requests, etc) are very much welcome.

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
  is _not_ accepted by the usage endpoint.

## Install

The project has two parts: a Go daemon/CLI and a Plasma applet. You can install
both with the Makefile, or each one on its own.

### Everything with the Makefile

```sh
git clone https://github.com/4ster-light/go-pane.git
cd go-pane
make install
systemctl --user daemon-reload
systemctl --user enable --now go-pane.service
```

`make install` puts the binary in `~/.local/bin`, the unit in
`~/.config/systemd/user`, and installs the applet.

### Daemon and CLI only

With the Go toolchain, no clone needed:

```sh
go install github.com/4ster-light/go-pane/cmd/go-pane@latest
```

This puts `go-pane` in `$(go env GOPATH)/bin` (usually `~/go/bin`), so make sure
that directory is on your `PATH`.

### Plasma applet only

From a git clone or an unpacked release tarball:

```sh
kpackagetool6 -t Plasma/Applet -i packaging/io.github.4ster-light.go-pane
```

Then right-click the panel, choose **Add Widgets…**, and add **OpenCode Go
Usage**.

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
- `GET /v1/history?limit=288` — raw samples for charts (oldest first)

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

`apiKey` is also accepted, but storing a secret in `config.json` is discouraged;
prefer `apiKeyFile` or an environment variable.

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
