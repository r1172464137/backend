# Fork releases and GHCR

## Source lines and production safety

- Production **v1.20.4** is tagged from `fix/reality-xver-v1.20.3`, based on upstream
  deployed v1.20.3 (`d0e31dd`). It adds REALITY xver and distribution fixes, not
  master migrations or unrelated runtime changes. Migration files are unchanged.
- `fix/reality-xver-master` separately forward-ports the same numeric JSON field
  `protocols[].reality_xver` / protobuf `ServerProtocol` tag 66 onto fork master
  `8b14c39`. That source line is NOT the v1.20.4 production deployment.
- Preserve database/config backups, mounted data, nginx routes and port mappings.
  Publishing an image does not upgrade any running server. Do not point a master
  binary/image at the production database as a substitute for the stable backport.

## Release assets

Fork: <https://github.com/r1172464137/backend/releases>. Tag `v1.20.4` injects
`1.20.4` into `github.com/perfect-panel/server/internal/app/buildinfo.Version`,
with `Channel=stable` and a single UTC RFC3339 build time across the run.
Release builders use Go **1.27.3**; the module's Go 1.27.1 directive is unchanged.

The existing release matrix is retained:

- `ppanel-server-linux-{386,amd64,arm64}.tar.gz`
- `ppanel-server-windows-{386,amd64,arm64}.zip`
- `ppanel-server-darwin-{amd64,arm64}.tar.gz`
- `install.sh` and `SHA256SUMS` covering all archives and the installer.

Each archive contains `ppanel-server` (`.exe` on Windows), LICENSE, an empty
`etc/ppanel.yaml` and the installer under a `ppanel-server-<os>-<arch>/` directory.
A locally initialized configuration is NEVER packaged. Updating an existing host
must preserve its own configuration instead of extracting the empty template over it.

Verify in a new directory before installing:

```bash
base=https://github.com/r1172464137/backend/releases/download/v1.20.4
curl --proto '=https' --tlsv1.2 -fSLO "$base/ppanel-server-linux-amd64.tar.gz"
curl --proto '=https' --tlsv1.2 -fSLO "$base/SHA256SUMS"
sha256sum --check --ignore-missing SHA256SUMS
```

For a new Linux/systemd installation, download `install.sh` from the SAME pinned
release, verify it against SHA256SUMS, inspect it, then run
`sudo env PPANEL_VERSION=v1.20.4 bash install.sh`. The script defaults to this fork;
`PPANEL_RELEASE_REPO=owner/repo` is an explicit override. It fails closed on failed
HTTP requests and missing/duplicate/malformed/mismatched checksums. It preserves
existing `etc/` and runtime data. It creates/refreshes a systemd unit and is not
intended to replace a Docker deployment. Master's installer retains its existing
unprivileged-user/PPANEL_DB/PPANEL_REDIS contract; the stable backport retains its
existing root service runtime, with an explicit `run --config` invocation.

Stable installer supports nonprivileged download-only verification:

```bash
PPANEL_VERSION=v1.20.4 bash script/install.sh --download-only ./verified-download
```

## Multiarch container

Image: **ghcr.io/r1172464137/ppanel-server**; platforms `linux/amd64,linux/arm64`.
For tagged commit `<sha>`, the release workflow publishes:

- `:1.20.4`
- `:v1.20.4`
- `:sha-<first-12-hex-of-tagged-commit>`
- `:1.20.4-<first-12-hex-of-tagged-commit>`

No ambiguous `latest`/`lts` floating tag is changed. OCI source/revision/version
labels identify this fork and the **tagged commit**, including manual recovery runs.
Pin by digest after validating a published image if immutability is required.

Release CI builds static binaries once on Go 1.27.3 runners, uses those identical
binaries in archives and `Dockerfile.release`, and exports/inspects BOTH platform
filesystems before pushing. `script/verify-release-rootfs.sh` compares binary bytes
and executable mode, verifies the CA bundle and timezone file are nonempty, and
requires an empty config template. No emulated core process is used for this check.

The runtime contract remains `/app/ppanel`, working directory `/app`, port 8080,
entrypoint `["/app/ppanel"]`, command `["run","--config","etc/ppanel.yaml"]`,
CA certificates at `/etc/ssl/certs/ca-certificates.crt`, and timezone Asia/Shanghai.
The stable Docker runtime is preserved. The separate master Dockerfile.release
retains master's pre-existing uid 65532, writable runtime directories and healthcheck;
none of those master behavior changes are shipped in the stable backport.

```bash
docker pull ghcr.io/r1172464137/ppanel-server:1.20.4
# Recreate only after validating the image; retain existing mounts, ports and nginx.
```

## CI triggers and permissions

`.github/workflows/release.yml` runs on pushed `v[0-9]*` tags (validated as `vX.Y.Z`,
optionally a prerelease suffix). `workflow_dispatch.release_tag` can rebuild an
existing tag. Builds/tests must pass before publishing a draft's complete set of
assets and SHA256SUMS; reruns replace the assets and recompute one consistent manifest.
Image publication follows asset publication and filesystem verification.

Only **GITHUB_TOKEN** is required: `contents: write` for Release assets and
`packages: write` for GHCR. No `GH_TOKEN` PAT or Docker Hub credentials are required.
Repository Actions must be enabled; pre-existing GHCR package ownership/ACLs may
need to allow this repository to publish. Main should verify the actual Release
assets, image platforms/labels, and anonymous package pulls after publication.

The concrete backend version/tool endpoint is not a self-updater: its GetVersion
implementation only returns `buildinfo.Display()` and has no upstream fetch.
No new runtime update API or remote download behavior was added. Keep using this
fork's pinned assets/images rather than a generic upstream update link.

## Local verification without live fixtures

```bash
bash script/test-release.sh
VERSION=1.20.4 CHANNEL=stable BUILD_TIME=2026-10-25T00:00:00Z bash script/release-build.sh linux amd64
# Go must resolve to the pinned Go 1.27.3 toolchain; build arm64 the same way.
```

Tests source the actual installer checksum helpers and mock curl locally, covering
valid checksums, corruption, missing/duplicate/malformed/path-traversal entries,
HTTP errors, repository/tag validation and rootfs validation failures. No service
installation, remote host access or physical core/network fixtures are needed.
