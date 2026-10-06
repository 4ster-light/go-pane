# Contributing to go-pane

Thanks for your interest! go-pane is primarily built for one person's KDE Plasma
6 desktop, but code quality is a core goal and contributions of any kind are
welcome: bug reports, feature requests, documentation, and code.

By participating you agree to abide by our
[Code of Conduct](CODE_OF_CONDUCT.md).

## Ways to contribute

- Report bugs and request features via
  [GitHub issues](https://github.com/4ster-light/go-pane/issues).
- Improve the docs (`README.md`, `PLAN.md`, this file).
- Send pull requests for fixes and features.
- Test on your own Plasma 6 setup and report environment differences.

## Toolchain

| Tool             | Version            | Purpose                                |
| ---------------- | ------------------ | -------------------------------------- |
| Go               | 1.23+ (see go.mod) | build the daemon and CLI               |
| golangci-lint    | v2.14.0            | linting (`make lint`)                  |
| KDE Plasma       | 6.x                | run the applet                         |
| `kpackagetool6`  | Plasma 6           | install/upgrade the plasmoid           |
| `plasmawindowed` | Plasma 6           | run the applet standalone (`make dev`) |

Optional:

- `qmllint` (from `qt6-qtdeclarative-devel` on Fedora) to lint QML.
- `plasmoidviewer` to preview the applet.

The Go module has **no external dependencies** and targets the standard library
only. Please keep it that way unless there is a strong justification; open an
issue to discuss first.

## Getting started

```sh
git clone https://github.com/4ster-light/go-pane.git
cd go-pane

make build             # -> bin/go-pane
make test              # unit tests
make check             # fmt-check + vet + tests + lint (run before pushing)
make run               # start the daemon
./bin/go-pane doctor   # verify API key discovery and connectivity
```

For applet work:

```sh
make dev               # kpackagetool6 -u + plasmawindowed
```

You need an OpenCode API key (`oc_sk_…`); see the README for the discovery
order. Never commit keys or other secrets.

## Project layout

```
cmd/go-pane/          CLI entry point (serve, once, json, doctor, version)
internal/opencode/    OpenCode Go API client
internal/metrics/     window model and derived metrics
internal/history/     JSONL sample store + learned window lengths
internal/server/      loopback HTTP API
internal/config/      flags, env, key discovery, XDG paths
internal/app/         poller that ties the pieces together
packaging/            systemd unit and the Plasma applet package
```

## Coding style

### Go

- Run `make fmt` (gofmt + goimports); CI enforces `make fmt-check`.
- Wrap errors with `%w` and inspect them with `errors.Is`/`errors.As`.
- Do not ignore errors. If a return value is intentionally discarded, assign it
  to `_` explicitly.
- Prefer table-driven tests, kept next to the code as `_test.go` files.
- Keep packages small and single-purpose under `internal/`.
- Pass dependencies explicitly; avoid package-level mutable state.
- Use `context.Context` for anything that performs I/O.
- Document every exported identifier.

### QML (plasmoid)

- Target Plasma 6 APIs (`PlasmoidItem`, `org.kde.kirigami`, …).
- **Never name a property `data` on an Item-derived type.** `Item.data` is the
  default property that holds visual children; shadowing it makes the applet
  occupy space but render nothing. Use `usage`/`snapshot` instead.
- Use `Kirigami.Units` for spacing and `Kirigami.Theme` for colours.
- Keep representations thin, all logic belongs in the Go daemon.
- Give compact representations `implicitWidth`/`implicitHeight` and matching
  `Layout.preferred*` bounds so the panel can size them.
- Validate with `qmllint` when available.

### Markdown and docs

- Wrap prose at ~80 columns.
- Use fenced code blocks with a language hint.
- Keep the README task-oriented; design detail belongs in `PLAN.md`.

## Testing

- Add or update tests for behaviour changes.
- Run `make test` locally; CI additionally runs `go test -race` with coverage.
- Prefer deterministic tests: inject a clock instead of sleeping.
- Put fixtures in `testdata/`.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope][!]: <description>

[optional body]

[optional footer(s)]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
`ci`, `chore`, `revert`.

Common scopes: `daemon`, `plasmoid`, `metrics`, `api`, `history`, `config`,
`server`, `cli`, `ci`, `docs`.

Rules:

- Imperative mood, lower case, no trailing period: `fix: handle 429 retries`.
- One logical change per commit.
- Mark breaking changes with `!` and a `BREAKING CHANGE:` footer.

Examples:

```
feat(plasmoid): show exhaustion ETA per window
fix(history): check bufio.Scanner.Err before using samples
docs: add contributing guide
ci: run tests with the race detector
```

## Pull requests

1. Fork and create a branch: `feat/…`, `fix/…`, or `docs/…`.
2. Make your change, add tests, and run `make check`.
3. Keep the PR focused; describe the what and why, and link related issues.
4. Ensure CI is green.
5. By submitting a PR you agree your contribution is licensed under the MIT
   License.

## Reporting bugs

Include your distribution, Plasma version, `go-pane version`, the output of
`go-pane doctor` (redact the key), and steps to reproduce.

For security issues, please use GitHub's private security advisories rather than
a public issue: <https://github.com/4ster-light/go-pane/security/advisories/new>

## License

go-pane is released under the [MIT License](LICENSE).
