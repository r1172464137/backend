#!/usr/bin/env bash
# Fork release installer/updater. Existing etc/ and runtime data are never replaced.
# PPANEL_RELEASE_REPO=owner/repo (default r1172464137/backend)
# PPANEL_VERSION=vX.Y.Z (default latest). --download-only DIR verifies without installing.
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then set -euo pipefail; fi

release_init() {
    RELEASE_REPO=$1; RELEASE_TAG=${2:-}
    [[ $RELEASE_REPO =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo 'invalid release repository' >&2; return 1; }
    if [[ -z $RELEASE_TAG ]]; then
        local effective
        effective=$(curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o /dev/null -w '%{url_effective}' "https://github.com/$RELEASE_REPO/releases/latest") || return 1
        [[ $effective == "https://github.com/$RELEASE_REPO/releases/tag/"* ]] || { echo 'latest release not found' >&2; return 1; }
        RELEASE_TAG=${effective##*/}
    fi
    [[ $RELEASE_TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]] || { echo 'invalid release tag (expected vX.Y.Z)' >&2; return 1; }
    RELEASE_BASE="https://github.com/$RELEASE_REPO/releases/download/$RELEASE_TAG"
}

release_fetch() {
    local asset=$1 dir=$2
    [[ $asset =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
    mkdir -p "$dir" || return 1
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$dir/SHA256SUMS" "$RELEASE_BASE/SHA256SUMS" || return 1
    # Exactly one entry, exactly this basename, and a SHA-256 digest. Never
    # feed untrusted manifest paths to sha256sum, or accept an empty selection.
    awk -v asset="$asset" '$2 == asset { n++; if (length($1) != 64 || $1 !~ /^[a-fA-F0-9]+$/ || NF != 2) bad=1; print $1 "  " asset } END { if (n != 1 || bad) exit 1 }' "$dir/SHA256SUMS" > "$dir/$asset.sha256" || { echo "invalid/missing checksum for $asset" >&2; return 1; }
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$dir/$asset" "$RELEASE_BASE/$asset" || return 1
    (cd "$dir" && sha256sum -c "$asset.sha256") || { rm -f "$dir/$asset"; return 1; }
}

# Sourcing exposes the actual production download helpers for nonprivileged tests.
if [[ ${BASH_SOURCE[0]} != "$0" ]]; then return 0; fi

version=${PPANEL_VERSION:-}; download_dir=''
while [[ $# -gt 0 ]]; do
    case $1 in
        --download-only) download_dir=${2:?directory required}; shift 2 ;;
        --help|-h) echo 'PPANEL_RELEASE_REPO=owner/repo PPANEL_VERSION=vX.Y.Z bash install.sh [--download-only DIR]'; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done
case $(uname -m) in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo 'unsupported architecture' >&2; exit 1 ;;
esac
release_init "${PPANEL_RELEASE_REPO:-r1172464137/backend}" "$version"
asset="ppanel-server-linux-$arch.tar.gz"
if [[ -n $download_dir ]]; then release_fetch "$asset" "$download_dir"; exit; fi
[[ $EUID == 0 && $(uname -s) == Linux ]] || { echo 'Linux root installation required; use --download-only for verification' >&2; exit 1; }
for cmd in curl tar sha256sum install systemctl; do command -v "$cmd" >/dev/null; done
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
release_fetch "$asset" "$tmp"
# Archives have one predictable top-level directory, not a layout guessed by find|head.
root=${asset%.tar.gz}
tar -tzf "$tmp/$asset" > "$tmp/entries"
awk -v root="$root" 'index($0, root "/") != 1 || $0 ~ /(^|\/)\.\.(\/|$)/ { bad=1 } END { exit bad }' "$tmp/entries"
mkdir "$tmp/extract"
tar -xzf "$tmp/$asset" --no-same-owner -C "$tmp/extract"
[[ -f $tmp/extract/$root/ppanel-server ]]
install_dir=/opt/ppanel-server
mkdir -p "$install_dir/etc"
if [[ -f $install_dir/ppanel-server ]]; then cp -p "$install_dir/ppanel-server" "$install_dir/ppanel-server.previous"; fi
# Stop only after the replacement has been downloaded and verified.
if systemctl is-active --quiet ppanel; then systemctl stop ppanel; fi
install -m 0755 "$tmp/extract/$root/ppanel-server" "$install_dir/ppanel-server"
if [[ ! -e $install_dir/etc/ppanel.yaml ]]; then install -m 0600 /dev/null "$install_dir/etc/ppanel.yaml"; fi
cat > /etc/systemd/system/ppanel.service <<EOF
[Unit]
Description=PPanel Server
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=$install_dir
ExecStart=$install_dir/ppanel-server run --config etc/ppanel.yaml
Restart=on-failure
RestartSec=5
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable ppanel
systemctl start ppanel
printf 'Installed %s from %s. Configuration preserved in %s/etc/ppanel.yaml\n' "$RELEASE_TAG" "$RELEASE_REPO" "$install_dir"
