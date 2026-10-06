# go-pane — OpenCode Go usage widget for KDE Plasma

A native KDE Plasma 6 applet that shows live OpenCode Go subscription usage
(rolling / weekly / monthly), remaining budget, reset countdowns, and derived
pacing metrics. The "brain" is written in Go; the applet is a thin QML
plasmoid that renders what the Go process exposes.

Status: **implemented (phases 1–3)** — Go core, daemon, HTTP API and the
Plasma applet are in place. See "Implementation status" at the end.

---

## 1. Executive summary

Plasma applets are QML packages loaded by `plasmashell`; they cannot be
implemented purely in Go. The pragmatic, robust design is therefore a **split**:

- **`go-pane` (Go daemon)** — authenticates to the OpenCode Go API, polls it,
  computes derived metrics, keeps a small history, and serves the result as
  JSON over loopback HTTP. Also usable as a one-shot CLI.
- **`go-pane` plasmoid (QML)** — a normal Plasma 6 applet that polls the
  daemon and renders compact + full representations and a config page.

This keeps all secrets, HTTP, retries, caching, math, and history in Go, while
the QML layer stays a dumb view. It is also testable end-to-end with `curl`.

---

## 2. Validated facts (probed on this machine, 2026-10-06)

| Thing | Value |
|---|---|
| OS / DE | Fedora 44, KDE Plasma **6.7.5** |
| Widget tooling | `kpackagetool6`, `plasmawindowed` present; `plasmoidviewer`/`qml6` **missing** (optional dev packages) |
| plasma5support | installed — `org.kde.plasma.plasma5support` QML module + `plasma_engine_executable.so` |
| Go | `go1.26.8 linux/amd64` |
| API base | `https://opencode.ai/zen/go` (Anthropic-style) and `https://opencode.ai/zen/go/v1` (OpenAI-style) |
| Usage endpoint | `GET https://opencode.ai/zen/go/v1/usage` |
| Auth | `Authorization: Bearer <oc_sk_…>` (an **API key**, not the OAuth `st_` token) |
| CORS | server sends `access-control-allow-origin: *` |
| Cache | `cache-control: no-store` |

Confirmed live response:

```json
{
  "usage": {
    "rolling": { "status": "ok", "percent": 0,  "resetsAt": "2026-10-06T13:39:32.000Z" },
    "weekly":  { "status": "ok", "percent": 20, "resetsAt": "2026-10-12T00:00:00.000Z" },
    "monthly": { "status": "ok", "percent": 57, "resetsAt": "2026-10-25T14:02:02.000Z" }
  }
}
```

Interpretation:

- `percent` = **percent used** (0–100). "Available left" = `100 - percent`.
- `rolling` window ≈ **5 hours** (13:39 − 08:40 ≈ 4h59m).
- `weekly` resets **Mondays 00:00 UTC**.
- `monthly` resets on the signup anniversary (varies 28–31 days); derive the
  exact window length from the previous `resetsAt` via history.
- `status` observed only as `"ok"`; handle unknown values gracefully.

Where the API key comes from on this machine:

- `~/.pi/agent/auth.json` → `{"opencode-go":{"type":"api_key","key":"oc_sk_…"}}`
- The key can also be supplied via `OPENCODE_API_KEY`.
- The OAuth credential stored by the opencode CLI in
  `~/.local/share/opencode/opencode.db` (`credential.value`, `st_…`/`rt_…`)
  is **rejected** by the usage endpoint (tested: HTTP 401). So the widget must
  use an `oc_sk_…` console API key.

---

## 3. Why Go + QML, not "all Go"

- Plasma 6 applets are QML packages (`metadata.json` + `contents/ui/*.qml`)
  executed inside `plasmashell`. There is no supported way to register a Go
  type into that QML engine without a C++ plugin.
- Qt bindings for Go (`miqt`, `therecipe/qt`) can build a standalone Qt
  window/tray app, but not an embeddable panel widget.
- Therefore: **Go owns the logic and data; QML owns the pixels.**

---

## 4. Architecture

```
┌──────────────────────────┐        GET /v1/usage (Bearer oc_sk_…)
│  OpenCode Go API         │◀────────────────────────────────────────┐
│  opencode.ai/zen/go/v1   │                                         │
└──────────────────────────┘                                         │
                                                                     │
┌───────────────────────────────────────────────┐                   │
│ go-pane daemon (Go, systemd --user)           │                   │
│  • key discovery + 0600 config                │                   │
│  • poller (30–60s) + exponential backoff      │                   │
│  • metrics engine (pace, projection, ETA)     │───────────────────┘
│  • history store (JSONL under XDG state)      │
│  • HTTP server 127.0.0.1:17873 (JSON + SSE)   │
└───────────────────────────────────────────────┘
             ▲  HTTP GET http://127.0.0.1:17873/v1/usage
             │  (QML XMLHttpRequest, every refresh interval)
┌────────────┴──────────────────────────────────┐
│ Plasma applet  io.github.<you>.go-pane         │
│  contents/ui/main.qml  (compact + full)        │
│  contents/ui/UsageRow.qml, Config*.qml         │
│  contents/config/main.xml                      │
└────────────────────────────────────────────────┘
```

Transport choice: **loopback HTTP**.
- Easy to debug (`curl`), no extra QML/DBus plumbing, CORS already permissive.
- Bind to `127.0.0.1` only. The endpoint exposes only percentages/countdowns,
  never the API key.
- Optional `--json` one-shot mode lets the plasmoid fall back to the
  `plasma5support` `executable` data engine (no daemon) if desired.

Fallbacks considered: DBus service (more native, more code), writing a state
file read via `file://` XHR (no live push, atomic-write hazards).

---

## 5. Repository layout

```
go-pane/
├── PLAN.md
├── README.md
├── LICENSE                       # MIT (suggested)
├── go.mod                        # module go-pane (or github.com/<you>/go-pane)
├── Makefile
├── cmd/
│   └── go-pane/
│       └── main.go               # serve | once | json | doctor | install | version
├── internal/
│   ├── config/config.go          # flags, env, key discovery, XDG paths
│   ├── opencode/client.go        # API client + retries + typed errors
│   ├── metrics/metrics.go        # window model + derived metrics
│   ├── metrics/metrics_test.go
│   ├── history/store.go          # append/read samples (JSONL), window-length learn
│   ├── server/server.go          # HTTP JSON + SSE, CORS, loopback guard
│   └── version/version.go
├── packaging/
│   ├── go-pane.service           # systemd --user unit
│   └── io.github.<you>.go-pane/  # the plasmoid KPackage
│       ├── metadata.json
│       └── contents/
│           ├── config/main.xml
│           ├── config/config.qml
│           └── ui/
│               ├── main.qml
│               ├── UsageRow.qml
│               └── ConfigGeneral.qml
└── testdata/
    └── usage.sample.json
```

Module path is a decision point — default to `go-pane` for a local build, or a
real `github.com/<you>/go-pane` if publishing.

---

## 6. Go daemon design

### 6.1 CLI

```
go-pane serve   [--addr 127.0.0.1:17873] [--interval 60s] [--api-key-file PATH]
go-pane once    [--json] [--pretty]        # one API call, print computed metrics
go-pane json    [--pretty]                 # alias of `once --json`
go-pane doctor                             # check key discovery + API reachability
go-pane install [--plasmoid] [--systemd]   # copy binary/unit, kpackagetool6 -i
go-pane version
```

`once --json` is important: it makes the binary usable as a
`plasma5support` "executable" data source and keeps the daemon optional for an
MVP.

### 6.2 Configuration & key discovery (first match wins)

1. `--api-key-file PATH`
2. `OPENCODE_API_KEY` / `OPENCODE_GO_API_KEY` env
3. `~/.config/go-pane/config.json` → `{"apiKeyFile": "…", "apiKey": "…"}`
4. `~/.config/go-pane/api_key` (file, must be `0600`)
5. `~/.pi/agent/auth.json` → `opencode-go.key`  ← works on this machine
6. error with actionable instructions (`go-pane doctor`)

Notes:
- Never log the key; redact in errors.
- Warn (once) if the key file is group/world-readable.
- Do **not** try the opencode CLI OAuth token — it 401s on this endpoint.

Config knobs (file + flags): refresh interval, port, history retention,
thresholds, monthly window override, whether to include free/zero-cost models.

### 6.3 API client (`internal/opencode`)

- `Client{http.Client, baseURL, apiKey}` with `FetchUsage(ctx) (Usage, error)`.
- Request: `GET {base}/zen/go/v1/usage`, `Authorization: Bearer`, `Accept: application/json`.
- 10s timeout; retry on 429/5xx/network with jittered exponential backoff
  (max ~4 attempts); honor `Retry-After` if present.
- Typed errors: `ErrUnauthorized`, `ErrRateLimited`, `ErrServer`, `ErrNetwork`
  so the UI can render distinct states.
- Parse `resetsAt` with `time.RFC3339` (Go accepts the `.000` fraction).

### 6.4 Poller & cache

- Goroutine loop; interval default **60s** (API is cheap; `no-store`).
- On success: update in-memory snapshot, append history sample, reset backoff.
- On failure: keep last-good snapshot, mark `stale=true`, expose `error`,
  increase backoff, keep serving.
- Single-flight so concurrent UI requests never fan out.

### 6.5 Metrics engine (`internal/metrics`)

See §8 for formulas. Pure functions over `(now, usage, learnedWindowLen,
previousSample)` → `Snapshot`. Fully unit-testable with an injected clock.

### 6.6 HTTP API (`internal/server`)

- `GET /healthz` → `200 ok`
- `GET /v1/usage` → latest `Snapshot` (below)
- `GET /v1/history?window=weekly&limit=288` → samples for sparklines
- `GET /v1/events` → optional SSE push (nice-to-have; polling is enough)
- Headers: `Access-Control-Allow-Origin: *`, `Cache-Control: no-store`.
- Reject non-loopback peers defensively (bind + check `RemoteAddr`).

`Snapshot` shape:

```json
{
  "fetchedAt": "2026-10-06T08:40:48Z",
  "stale": false,
  "error": null,
  "headline": "monthly",
  "windows": {
    "rolling": {
      "status": "ok",
      "usedPercent": 0,
      "availablePercent": 100,
      "resetsAt": "2026-10-06T13:39:32Z",
      "windowSeconds": 18000,
      "elapsedPercent": 2.1,
      "remainingSeconds": 17900,
      "paceRatio": 0.0,
      "projectedUsedPercent": 0.0,
      "exhaustsAt": null,
      "safeRatePerHour": 20.0,
      "budgetPerHour": 20.0
    },
    "weekly":  { "...": "..." },
    "monthly": { "...": "..." }
  }
}
```

### 6.7 History store (`internal/history`)

- Append-only JSONL at `$XDG_STATE_HOME/go-pane/history.jsonl`
  (default `~/.local/state/go-pane/history.jsonl`).
- One line per successful poll: `{ts, rolling, weekly, monthly, resetsAt…}`.
- Rotate/trim to a retention window (e.g. 60 days) on startup.
- Uses: sparklines, `deltaSinceLast`, and **learning the true window length**:
  when `resetsAt` changes, `windowLen = newResetsAt - oldResetsAt`; persist it
  per window. First run falls back to defaults (rolling 5h, weekly 7d,
  monthly 30d) and marks derived pace as "estimated".
- No SQLite needed. If richer queries are ever required, switch to
  `modernc.org/sqlite` (pure Go, no cgo).

### 6.8 systemd user unit

`packaging/go-pane.service`:

```ini
[Unit]
Description=OpenCode Go usage poller
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/go-pane serve
Restart=on-failure
RestartSec=5
Environment=GO_PANE_LOG_LEVEL=info

[Install]
WantedBy=default.target
```

`systemctl --user enable --now go-pane.service`. The plasmoid may offer a
"Start daemon" button that runs the same command through the executable data
engine if the socket is unreachable.

---

## 7. Plasmoid design

### 7.1 `metadata.json`

```json
{
  "KPackageStructure": "Plasma/Applet",
  "KPlugin": {
    "Id": "io.github.<you>.go-pane",
    "Name": "OpenCode Go Usage",
    "Description": "Live OpenCode Go rolling/weekly/monthly usage, resets and pacing",
    "Icon": "office-chart-line",
    "Category": "System Information",
    "Version": "1.0.0",
    "License": "MIT",
    "Authors": [{ "Name": "<you>" }],
    "Website": "https://github.com/<you>/go-pane"
  },
  "X-Plasma-API-Minimum-Version": "6.0"
}
```

### 7.2 QML files

- `main.qml`
  - `import org.kde.plasma.plasmoid`, `org.kde.plasma.core as PlasmaCore`,
    `org.kde.plasma.components as PlasmaComponents3`,
    `org.kde.plasma.extras as PlasmaExtras`, `org.kde.kirigami as Kirigami`.
  - `Plasmoid.compactRepresentation` and `Plasmoid.fullRepresentation`.
  - `Timer` + `XMLHttpRequest` GET `http://127.0.0.1:17873/v1/usage`.
  - `Plasmoid.status` / `Plasmoid.busy` reflect `stale`/`error`.
  - `Plasmoid.toolTipMainText`/`toolTipSubText` for the panel tooltip.
- `UsageRow.qml` — reusable per-window row: label, progress bar, `% left`,
  reset countdown, pace badge, optional sparkline.
- `ConfigGeneral.qml` — daemon URL/port, refresh interval, thresholds,
  which windows to show, compact mode, "start daemon" button.

### 7.3 Representations

**Compact (panel):** three slim stacked bars (rolling/weekly/monthly) with the
most-constrained window's `% left` as the headline; color by threshold. Option:
single ring gauge.

**Full (popup):** per window a row:

```
Monthly             43% left · used 57%
[██████████████████░░░░░░░░░░]   resets in 19d 5h
pace 0.95×  ·  safe 1.8%/h  ·  projected 71% at reset
```

Header shows last update + a refresh button; stale/error states get a banner.

### 7.4 Config (`contents/config/main.xml`)

`String daemonUrl` (`http://127.0.0.1:17873`), `Int refreshSeconds` (60),
`Int warnPercent` (70), `Int criticalPercent` (90), `Bool showRolling/Weekly/
Monthly`, `String compactMode` (`constrained`|`monthly`|`rolling`), `Bool
notifyOnThreshold`.

The API key is **not** stored in KConfig — it lives only in the Go config file
(`0600`) or the env. The config page shows the resolved key **source** (not the
value) via the daemon's `/v1/usage` metadata.

---

## 8. Metrics spec

Per window:

| Metric | Formula | Display |
|---|---|---|
| used | `percent` | `57% used` |
| available | `100 - percent` | `43% left` |
| resets in | `resetsAt - now` | `19d 5h`, `4h 59m` |
| window | learned, else default (5h / 7d / 30d) | tooltip |
| elapsed % | `clamp((now - (resetsAt - window)) / window * 100)` | tooltip |
| pace ratio | `(used/100) / (elapsed/100)` | `0.95× pace` |
| projection | `(used/100) / elapsedFrac * 100` | `projected 71% at reset` |
| exhaust ETA | `now + elapsed * (1 - usedFrac) / usedFrac` | `empty in ~2h 10m` (only if `< resetsAt`) |
| safe rate | `available / remainingHours` | `1.8%/h` |
| budget rate | `100 / windowHours` | tooltip |
| status color | green `<warn`, amber `≥warn`, red `≥critical` | bar + badge |

Edge cases: `elapsedFrac == 0` → pace undefined ("just reset"); `used == 0` →
no ETA; `status != ok` → override color/text; stale snapshot → grey + age.

**Headline selection:** the window with the highest `projectedUsedPercent`
(tie-break highest `used`). Configurable.

Human formatting helpers: `formatDuration` (days+hours, or hours+minutes under
24h) and `formatPercent` (round to int unless <1%).

---

## 9. Build & install

Makefile targets:

```
make build            # go build -o bin/go-pane ./cmd/go-pane
make test             # go test ./...
make run              # go run ./cmd/go-pane serve
make plasmoid-install # kpackagetool6 -t Plasma/Applet -i packaging/io.github.<you>.go-pane
make plasmoid-upgrade # kpackagetool6 -t Plasma/Applet -u packaging/io.github.<you>.go-pane
make install          # binary -> ~/.local/bin, unit -> ~/.config/systemd/user, plasmoid
make uninstall
make dev              # plasmawindowed io.github.<you>.go-pane
```

Optional dev packages for QML validation: `qt6-qtdeclarative-devel` (provides
`qml6`/`qmllint`) and `plasma-workspace`'s `plasmoidviewer` if available.

---

## 10. Phased roadmap

- **Phase 0 — validate** ✅ done: endpoint, auth, schema, window cadence, tooling.
- **Phase 1 — Go core (no UI):** client + metrics + `go-pane once --json`,
  unit tests, golden file. Exit: correct JSON on this machine.
- **Phase 2 — daemon:** poller, cache/backoff, HTTP API, config/key discovery,
  history + window-length learning, systemd unit. Exit: `curl` shows live,
  resilient data.
- **Phase 3 — plasmoid MVP:** metadata, compact + full, three bars, countdown,
  polling, tooltip. Exit: added to panel and updating.
- **Phase 4 — config & polish:** config page, thresholds, start-daemon button,
  stale/error UI, install targets.
- **Phase 5 — insights:** sparklines, deltas, threshold notifications,
  "projected to exceed" warnings.
- **Phase 6 — packaging/release:** README, screenshots, CI (`go test`, `go vet`,
  optional `qmllint`), GitHub release tarball, KDE Store submission.

---

## 11. Testing

- **Go unit:** metrics with a fixed clock (pace, projection, ETA, window
  learning, DST/UTC); client via `httptest` (401/429/5xx/backoff); key
  discovery precedence; history rotation.
- **Golden:** `testdata/usage.sample.json` → expected snapshot JSON.
- **Integration:** run daemon on an ephemeral port, hit `/v1/usage` + `/healthz`.
- **Manual QML:** `make dev` (`plasmawindowed`), then add to a panel; verify
  compact/full, tooltip, config apply, daemon-down state.
- **CI:** `go test ./...`, `go vet ./...`, `gofmt -l`; run `qmllint` if the
  runner has Qt dev tools.

---

## 12. Risks, assumptions & decisions to confirm

1. **Unofficial endpoint.** `/zen/go/v1/usage` is undocumented and may change
   or rate-limit. Mitigate: isolate in `internal/opencode`, make it
   configurable, degrade gracefully, pin behavior tests.
2. **Window lengths are inferred.** `rolling` ≈ 5h and `weekly` = Mon 00:00 UTC
   are observations; `monthly` is an anniversary. Learning from history is the
   safe approach; defaults are documented as estimates.
3. **Key type.** Only `oc_sk_…` works; OAuth `st_…` does not. Document clearly.
4. **Naming/module path** (`go-pane` vs `github.com/<you>/go-pane`) and
   plasmoid id (`io.github.<you>.go-pane`) need your input.
5. **Daemon lifecycle:** systemd unit vs plasmoid-managed process. Recommend
   systemd; keep one-shot fallback.
6. **Multiple panels/instances:** one shared daemon serves all applets.
7. **Time zones:** all math in UTC from `resetsAt`; display in local time.

---

## 13. Alternatives considered

| Option | Verdict |
|---|---|
| Pure Go Qt app (`miqt`/`therecipe`) | Not an embeddable plasmoid; standalone window/tray only. |
| DBus daemon + QML `DBusInterface` | More native, more code/glue; HTTP is easier to debug. |
| `plasma5support` `executable` engine only (no daemon) | Simple MVP, but re-spawns per poll, no history/cache. Keep as fallback. |
| QML calls the API directly | Works (CORS `*`) but leaks the key into KConfig, no history/math in Go. Rejected. |
| State file + `file://` XHR | No live push; atomic-write hazards. Rejected as primary. |

---

## 14. Security note

The API key is a bearer credential. The daemon must keep it out of logs, out of
the HTTP responses, and out of KConfig; the key file should be `0600`. Also
note that pi stores it in `~/.pi/agent/auth.json` and it has appeared in pi
session transcripts — if any key was echoed during development, **rotate it**
in the OpenCode console.

---

## 15. Implementation status

Implemented:

- `internal/opencode` — typed client, retries live in the caller, error kinds.
- `internal/metrics` — window model, pace/projection/ETA/safe-rate, formatting.
- `internal/history` — JSONL samples + learned window lengths (with sanity
  bounds so missed resets don't corrupt the cadence).
- `internal/server` — loopback-only HTTP with CORS, `/healthz`, `/v1/usage`,
  `/v1/history`.
- `internal/config` — key discovery (env → dedicated file → pi auth), XDG paths.
- `internal/app` — poller with backoff and stale snapshot handling.
- `cmd/go-pane` — `serve`, `once`/`json`, `doctor`, `version`.
- `packaging/io.github.4ster-light.go-pane` — the Plasma 6 applet (compact,
  full, usage rows, config page) with `metadata.json` + KConfigXT.
- `packaging/go-pane.service`, `Makefile`, `README.md`, `LICENSE`.
- Tests for metrics, client, history and config. All pass; `go vet` clean.
- Verified against the live API; applet installed via `kpackagetool6` and the
  user service enabled.

Deviations from the plan:

- `server.Provider.History` takes only `limit` (the window filter happens in
  the client, since a sample already contains every window).
- One-shot mode also records a history sample so it can learn window lengths
  when used without the daemon.
- No SSE endpoint yet; the applet polls, which is sufficient at 60s.

Next (phase 4+): config-page polish, sparklines, threshold notifications,
GitHub CI, KDE Store packaging.
