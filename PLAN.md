# go-pane — design notes

A native KDE Plasma 6 applet that shows live OpenCode Go subscription usage
(rolling / weekly / monthly), remaining budget, reset countdowns and derived
pacing metrics.

As of **v0.2.0** the applet is self-contained: everything runs inside
`contents/ui/main.qml` (plus the KConfigXT schema). There is no daemon, no CLI
and no Go code.

---

## 1. Architecture

```
┌──────────────────────────────┐   GET /zen/go/v1/usage
│  OpenCode Go API             │◀─────────────────────────────┐
│  opencode.ai/zen/go/v1      │   Authorization: Bearer oc_sk_…
└──────────────────────────────┘
                                                               │
┌──────────────────────────────────────────────────────────────┴─────┐
│ Plasma applet  io.github.4ster-light.go-pane                       │
│  contents/ui/main.qml                                              │
│    • KConfig-backed settings (API key, base URL, thresholds, …)    │
│    • XMLHttpRequest polling on a Timer                             │
│    • metrics engine (pace, projection, ETA, safe rate) in JS       │
│    • compact panel + detailed popup representations                │
│  contents/config/main.xml  (defaults / key schema)                 │
└────────────────────────────────────────────────────────────────────┘
```

There is exactly one QML file. Plasma requires the configuration *schema* to be
declared in `contents/config/main.xml`, which is XML rather than QML; the
settings **UI** is built into the popup, so no extra config QML is needed.

The API sends `access-control-allow-origin: *`, so a direct `XMLHttpRequest`
from the applet works.

---

## 2. Configuration (`contents/config/main.xml`)

| Key                | Type   | Default                          |
| ------------------ | ------ | -------------------------------- |
| `apiKey`           | String | *(empty)*                        |
| `baseUrl`          | String | `https://opencode.ai/zen/go/v1`  |
| `refreshSeconds`   | Int    | `60` (5–3600)                    |
| `warnPercent`      | Int    | `70`                             |
| `criticalPercent`  | Int    | `90`                             |
| `showRolling`      | Bool   | `true`                           |
| `showWeekly`       | Bool   | `true`                           |
| `showMonthly`      | Bool   | `true`                           |
| `compactMode`      | String | `constrained`                    |

`compactMode` is one of `constrained`, `monthly`, `rolling`.

Writes go straight to `Plasmoid.configuration`, which persists to the applet's
KConfig group (a `0600` file) immediately.

---

## 3. Metrics spec

Per window, with `now` the current time, `used = percent`, `resetsAt` from the
API and `windowLen` from the table below:

| Metric        | Formula                                                          |
| ------------- | ---------------------------------------------------------------- |
| available     | `100 - used`                                                     |
| window start  | `resetsAt - windowLen`                                           |
| elapsed       | `clamp(now - windowStart, 0, windowLen)`                         |
| elapsed %     | `elapsed / windowLen * 100`                                      |
| resets in     | `max(0, resetsAt - now)`                                         |
| pace ratio    | `(used/100) / (elapsed/windowLen)` (undefined at `elapsed == 0`) |
| projection    | `paceRatio * 100`                                                |
| exhaust ETA   | `now + elapsed * (1 - usedFrac) / usedFrac` when `usedFrac > 0`  |
| safe rate     | `available / remainingHours` while `remaining > 0`               |
| budget rate   | `100 / windowHours`                                              |
| status colour | green `< warn`, amber `≥ warn`, red `≥ critical`                 |

**Window lengths** are fixed estimates because the pure-QML design keeps no
history:

- rolling: **5 h** (observed ≈ 4h59m)
- weekly: **7 d** (resets Monday 00:00 UTC)
- monthly: **30 d** (a signup anniversary, 28–31 d)

**Headline selection:** the window with the highest projected usage (falling
back to used % when there is no projection); ties keep display order.

**Formatting:** durations render as `3d 4h`, `4h 59m` or `12m`; percentages
round to an integer except below 1 %, which keeps one decimal.

---

## 4. Fetching and error handling

- `Timer` with `interval = refreshSeconds`, `triggeredOnStart: true`.
- `XMLHttpRequest` GET `${baseUrl}/usage` with a 15 s timeout and the bearer
  header.
- HTTP 200 → parse `{ "usage": { rolling, weekly, monthly } }` and recompute.
- 401/403 → "API key rejected"; 429 → "Rate limited"; 0/network → "API not
  reachable"; anything else → "Request failed (HTTP n)".
- On error the last good snapshot is kept and the message is shown in the
  popup and tooltip.

The API response shape is:

```json
{
  "usage": {
    "rolling": { "status": "ok", "percent": 0,  "resetsAt": "2026-10-06T13:39:32.000Z" },
    "weekly":  { "status": "ok", "percent": 20, "resetsAt": "2026-10-12T00:00:00.000Z" },
    "monthly": { "status": "ok", "percent": 57, "resetsAt": "2026-10-25T14:02:02.000Z" }
  }
}
```

`percent` is **percent used**; `available = 100 - percent`.

---

## 5. Security

The API key is a bearer credential. In the daemonless design it lives in the
widget's KConfig, which is created `0600` but is not encrypted. This is the
trade-off accepted in exchange for removing the daemon. Rotate the key in the
OpenCode console if it leaks.

---

## 6. Validation

`scripts/validate_package.py` (run by `make check` and CI) checks:

- the package layout (`metadata.json`, `main.xml`, `main.qml`);
- `metadata.json` is valid JSON with a `MAJOR.MINOR.PATCH` version;
- `main.xml` is valid KConfigXT XML;
- every `Plasmoid.configuration.<key>` used in the QML is declared in
  `main.xml` (and warns about declared-but-unused keys).

`make dev` upgrades the package and opens it in `plasmawindowed` for manual
checks: compact/full representations, tooltip, config apply and the error
state. `qmllint` can be run when the Qt dev tools are installed.

---

## 7. Why not a daemon (history)

v0.1.0 shipped a Go daemon that polled the API, kept JSONL history, learned the
real window lengths from reset timestamps and served JSON over loopback HTTP to
the applet. It was robust but heavyweight for a single-user widget.

Because the API is CORS-permissive and the only state the applet needs is a
handful of derived numbers, the daemon was removed in v0.2.0 and its logic
folded into one QML file. What was lost:

- exact window-length learning (defaults are used instead);
- history/sparklines and delta tracking;
- keeping the key out of KConfig;
- the `doctor`/`once` CLI and the loopback HTTP API.

What was gained:

- one file to read and install, no systemd unit, no Go build, no port;
- fewer moving parts and a simpler key setup (paste it into the widget).

The full Go implementation remains available in git history at tag `v0.1.0`.
