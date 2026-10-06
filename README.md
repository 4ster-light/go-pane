# go-pane

[![CI](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml/badge.svg)](https://github.com/4ster-light/go-pane/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/4ster-light/go-pane)](https://github.com/4ster-light/go-pane/releases/latest)
[![License: MIT](https://img.shields.io/github/license/4ster-light/go-pane)](LICENSE)
[![KDE Plasma 6](https://img.shields.io/badge/KDE%20Plasma-6-blue)](https://kde.org/plasma-desktop/)

> [!NOTE]
> This project is primarily for personal use, hence the specific target
> environment. However, codebase quality is also a main goal and it should work
> in any compatible environment (Plasma 6 KDE) and contributions of any kind
> (issues, feature requests, etc) are very much welcome.

A native KDE Plasma 6 widget that shows live **OpenCode Go** subscription usage
(rolling / weekly / monthly) with reset countdowns and derived pacing metrics.

As of **v0.2.0** the whole thing is a single, self-contained QML plasmoid. There
is no daemon, no helper process and no Go toolchain: the applet calls the
OpenCode Go usage API directly and derives every metric in JavaScript.

```
OpenCode Go API ──> Plasma applet (single main.qml + KConfig)
```

## Features

- Rolling, weekly and monthly usage, matching the OpenCode dashboard.
- **Available % left** and **reset countdown** (`19d 5h`, `4h 59m`, …).
- Derived metrics: pace ratio, projected usage at reset, exhaustion ETA and a
  safe `%/h` budget.
- Compact panel representation (three bars + most-constrained headline) and a
  detailed popup.
- Configurable API key, base URL, refresh interval, warning/critical
  thresholds, visible windows and panel headline, all from the widget's popup.
- Stale/error states and a one-click refresh.

## Requirements

- KDE Plasma 6 (`kpackagetool6`)
- An OpenCode **API key** (`oc_sk_…`). The OAuth token used by the opencode CLI
  is _not_ accepted by the usage endpoint.

Go is **not** required anymore.

## Install

Download `go-pane-0.2.0.plasmoid` from the
[latest release](https://github.com/4ster-light/go-pane/releases/latest) and
install it:

```sh
kpackagetool6 -t Plasma/Applet -i go-pane-0.2.0.plasmoid
```

Or build it from a git clone:

```sh
git clone https://github.com/4ster-light/go-pane.git
cd go-pane
make install
```

`make package` builds the `.plasmoid` archive from the sources if you prefer to
install that way.

Then right-click the panel, choose **Add Widgets…**, and add **OpenCode Go
Usage**. The settings open automatically the first time; paste your API key and
the widget starts polling.

To upgrade an existing install:

```sh
make plasmoid-upgrade
```

## Configuration

Open the widget's popup and click the gear (⚙). Settings are saved in the
widget's Plasma configuration (a `0600` file):

| Setting         | Default                          | Description                                     |
| --------------- | -------------------------------- | ----------------------------------------------- |
| API key         | *(empty)*                        | `oc_sk_…` bearer key for the usage endpoint.    |
| API base URL    | `https://opencode.ai/zen/go/v1`  | Change if the endpoint moves.                   |
| Refresh (s)     | `60`                             | Poll interval, 5–3600 seconds.                  |
| Warn / critical | `70` / `90`                      | Used-% thresholds for amber / red.              |
| Show            | rolling, weekly, monthly         | Which rows appear in the popup.                 |
| Panel headline  | most constrained                 | Which window the panel emphasises.              |

## API key discovery

The key is stored in the widget's own Plasma configuration. There is no
environment variable or key-file lookup in the QML-only design; if you prefer to
keep the key out of KConfig, use v0.1.0 (the Go daemon) instead.

## Notes

- The usage endpoint (`GET /zen/go/v1/usage`) is undocumented and may change;
  the base URL is configurable.
- Window lengths are inferred and fixed: rolling ≈ 5h, weekly 7d (Monday 00:00
  UTC) and monthly 30d (a signup anniversary). Pacing for the monthly window is
  therefore an estimate. The old daemon learned exact lengths from history.
- The API sends `access-control-allow-origin: *`, which is what allows the QML
  `XMLHttpRequest` to call it directly.

## Security

The API key is a bearer credential and is written to the widget's KConfig in
your Plasma configuration directory. That file is created `0600`, but it is not
encrypted. Rotate the key in the OpenCode console if it leaks.

## Development

```sh
make dev        # upgrade the applet and open it with plasmawindowed
make check      # static package validation (metadata, KConfigXT, QML refs)
make package    # build dist/go-pane-<version>.plasmoid
```

There is no prebuilt binary or test suite: the widget *is* the single
`contents/ui/main.qml`. Optional dev packages for validation: `qmllint` (from
`qt6-qtdeclarative-devel` on Fedora).

CI runs `scripts/validate_package.py`, which checks the package layout, the
`metadata.json`, the KConfigXT schema and that every configuration key used in
the QML is declared.

## License

MIT
