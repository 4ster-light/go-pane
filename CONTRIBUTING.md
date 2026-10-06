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

| Tool             | Version   | Purpose                              |
| ---------------- | --------- | ------------------------------------ |
| KDE Plasma       | 6.x       | run the applet                       |
| `kpackagetool6`  | Plasma 6  | install/upgrade the plasmoid         |
| `plasmawindowed` | Plasma 6  | run the applet standalone (`make dev`) |
| Python           | 3.9+      | run the package checks (`make check`) |

Optional:

- `qmllint` (from `qt6-qtdeclarative-devel` on Fedora) to lint QML.

There is no Go toolchain and no build step: the applet is QML that Plasma loads
directly.

## Getting started

```sh
git clone https://github.com/4ster-light/go-pane.git
cd go-pane

make check    # metadata, KConfigXT and QML configuration-key checks
make dev      # install/upgrade + plasmawindowed
```

You need an OpenCode API key (`oc_sk_…`); see the README. Never commit keys or
other secrets.

## Project layout

```
packaging/io.github.4ster-light.go-pane/
  metadata.json                 Plasma applet metadata
  contents/config/main.xml      KConfigXT schema and defaults
  contents/ui/main.qml          the entire applet (UI + API + metrics)
scripts/validate_package.py     static package checks used by CI
```

## Coding style

### QML (plasmoid)

- Target Plasma 6 APIs (`PlasmoidItem`, `org.kde.kirigami`, …).
- Keep everything in `contents/ui/main.qml`. If a change really needs another
  file, justify it in the PR.
- **Never name a property `data` on an Item-derived type.** `Item.data` is the
  default property that holds visual children; shadowing it makes the applet
  occupy space but render nothing. Use `usage`/`snapshot` instead.
- Use `Kirigami.Units` for spacing and `Kirigami.Theme` for colours.
- Give compact representations `implicitWidth`/`implicitHeight` and matching
  `Layout.preferred*` bounds so the panel can size them.
- Declare every configuration key in `contents/config/main.xml`; the validator
  fails the build otherwise.
- Local file access via `XMLHttpRequest` is disabled in Qt. To read a file, use
  the `plasma5support` executable data engine (see key discovery in `PLAN.md`).
- Validate with `qmllint` when available and `make check` always.

### Python

- `scripts/` is tooling, not shipped. Standard library only, `python3`.
- Keep the validator dependency-free so CI can run it anywhere.

### Markdown and docs

- Wrap prose at ~80 columns.
- Use fenced code blocks with a language hint.
- Keep the README task-oriented; design detail belongs in `PLAN.md`.

## Testing

- There is no unit-test framework for QML here; run `make check` for static
  validation.
- For behaviour changes, test manually with `make dev`: compact and full
  representations, tooltip, settings apply, and the error/stale state (use a bad
  API key or base URL).

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope][!]: <description>

[optional body]

[optional footer(s)]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
`ci`, `chore`, `revert`.

Common scopes: `plasmoid`, `metrics`, `config`, `ci`, `docs`.

Rules:

- Imperative mood, lower case, no trailing period: `fix: handle 429 responses`.
- One logical change per commit.
- Mark breaking changes with `!` and a `BREAKING CHANGE:` footer.

Examples:

```
feat(plasmoid): show exhaustion ETA per window
fix(config): keep the key field in sync when settings reopen
docs: add contributing guide
ci: validate the package layout
```

## Pull requests

1. Fork and create a branch: `feat/…`, `fix/…`, or `docs/…`.
2. Make your change and run `make check`.
3. Keep the PR focused; describe the what and why, and link related issues.
4. Ensure CI is green.
5. By submitting a PR you agree your contribution is licensed under the MIT
   License.

## Reporting bugs

Include your distribution, Plasma version, and steps to reproduce. Do **not**
paste your API key; redact it as `oc_sk_…`.

For security issues, please use GitHub's private security advisories rather than
a public issue: <https://github.com/4ster-light/go-pane/security/advisories/new>

## License

go-pane is released under the [MIT License](LICENSE).
