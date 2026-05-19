# Release Checklist

`tmact` is packaged for clone-and-install use and tag-based GitHub Releases.

## Before Sharing

- Run `go test ./...`.
- Run `go build -o .cache/tmact ./cmd/tmact`.
- Confirm `README.md` quick-start commands match the intended install path.
- Confirm examples do not contain private paths, pane names, or machine-local
  details.
- Confirm the license file matches the intended audience.

## Clone Install

```sh
git clone <repo-url>
cd tmact
scripts/install.sh --bin-only
tmact ls
```

On macOS, install the status daemon:

```sh
scripts/install.sh
launchctl print "gui/$(id -u)/com.tmact.statusd"
```

Use `TMACT_WEB_ADDR=0.0.0.0:7890 scripts/install.sh` only on a trusted network.

## Release Binary

Push a `v*` tag to build macOS release archives and create a GitHub Release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow publishes:

- `tmact_darwin_arm64.tar.gz`
- `tmact_darwin_amd64.tar.gz`
- `checksums.txt`

Install the latest macOS release binary:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | sh
```

Install a specific tag:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_VERSION=v0.1.0 sh
```

Install the release binary plus the macOS LaunchAgent:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_INSTALL_STATUSD=1 sh
```

Use `TMACT_WEB_ADDR=0.0.0.0:7890` with the release installer only on a trusted
network.

## Public Release Prep

- Confirm `LICENSE` and release metadata still match the intended license.
- Decide whether Homebrew or `go install` should be supported in addition to
  GitHub Release binaries.
- Tag releases with a clear version such as `v0.1.0`.
- Include validation output and any live-tmux smoke testing in release notes.
