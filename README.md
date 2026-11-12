# CrumbCLI

Read selected browser cookies as JSON on stdout. A small Linux CLI built with Go and [Kooky](https://github.com/browserutils/kooky).

## Install and use

Build with the toolchain declared in [go.mod](go.mod):

```sh
go install github.com/zewelor/crumbcli/cmd/crumbcli@latest
crumbcli --browser brave-origin --domain x.com
crumbcli --browser brave-origin --domain x.com --domain twitter.com --name auth_token --name ct0
```

Run as the desktop user who owns the browser profile, with access to its unlocked keyring/session bus.

`--browser` and at least one `--domain` are required. Domains include their subdomains; unrelated suffixes such as `evilx.com` are excluded. Repeatable `--name` filters are optional. Only available matches are returned. Stdin is unused.

Output is a JSON array containing cookie values, sorted by domain/path/name:

```json
[{"name":"session","value":"synthetic-example","domain":".example.com","path":"/","secure":true,"httpOnly":true}]
```

Values, including empty strings, Unicode and JSON text, are preserved. Invalid UTF-8 in any selected cookie field fails with exit code 1 before any JSON is written, instead of silently replacing bytes. Expired cookies are omitted. No matches returns `[]` with exit code 0. Usage errors exit 2; read/output failures exit 1. Diagnostics use stderr and never include cookie values. Ctrl+C and SIGTERM terminate the process using OS defaults. An interrupted or failed output stream can contain partial JSON.

The consumer validates cookies before constructing HTTP requests. CrumbCLI does not write to Vault, upload cookies or persist exports.

## Browsers and profiles

Supported browsers: `brave-origin`, `brave`, `chrome`, `chromium`.

Chromium-family browsers use `Default`, or the directory name supplied with `--profile 'Profile 1'`. Brave Origin uses `BraveSoftware/Brave-Origin`; `XDG_CONFIG_HOME` is respected.

For a custom, Flatpak or Snap location, use `--profile-dir DIR` instead of `--profile`:

```sh
crumbcli --browser brave-origin --profile-dir "$HOME/custom-browser-profile/Default" --domain example.com
```

`HOME` is needed only when resolving paths below your home directory. Missing or ambiguous databases fail instead of switching accounts. Close the browser if you need its in-memory state flushed to disk. Only unpartitioned cookies are supported; the output cannot preserve partition scope, and the current reader does not reliably expose it. Other attributes beyond the six output fields are outside this MVP.

## Development

Application code and tests live in [cmd/crumbcli](cmd/crumbcli/). To install from a checkout, run `go install ./cmd/crumbcli`.

```sh
just hooks
just ci
just dist
```

`just ci` runs formatting, vet, race tests, vulnerability analysis, binary E2E and module verification. Python 3 E2E exercises all four adapters against a synthetic Chromium SQLite profile with plaintext values; `artifacts/e2e.json` contains scenario results only. Fixtures do not prove live Brave/Chrome keyring compatibility or upstream session validity.

`just dist` builds Linux amd64/arm64 archives and checksums. Hooks run fast checks before commit and the full gate before push. GitHub CI uploads build artifacts; pushing a `vMAJOR.MINOR.PATCH` tag publishes verified archives. [Renovate](.github/renovate.json) can automerge GitHub Actions updates after CI; other dependencies require review. `crumbcli --help` and `crumbcli version` require no profile. Local agent skills are described in [AGENTS.md](AGENTS.md).
