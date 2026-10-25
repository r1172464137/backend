#!/usr/bin/env bash
# Nonprivileged tests of the production download helpers. No HTTP or service calls.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "$here/install.sh"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/source" "$tmp/download"
printf 'verified release payload\n' > "$tmp/source/test.zip"
(cd "$tmp/source" && sha256sum test.zip > SHA256SUMS)
cp "$tmp/source/SHA256SUMS" "$tmp/good-sums"

curl() {
    local output='' effective=false arg url=''
    while [[ $# -gt 0 ]]; do
        arg=$1; shift
        case $arg in
            -o) output=$1; shift ;;
            -w) effective=true; shift ;;
            --proto|--tlsv1.2|--retry) [[ $arg == --tlsv1.2 ]] || shift ;;
            https:*) url=$arg ;;
        esac
    done
    [[ ${CURL_FAIL:-false} == false ]] || return 22
    [[ $url == https://github.com/test/fork/releases/* ]] || return 23
    if $effective; then printf '%s' 'https://github.com/test/fork/releases/tag/v1.2.3'; return; fi
    cp "$tmp/source/${url##*/}" "$output"
}

release_init test/fork ''
[[ $RELEASE_TAG == v1.2.3 ]]
release_fetch test.zip "$tmp/download"
cmp "$tmp/source/test.zip" "$tmp/download/test.zip"
if release_init '../bad/repo' v1.2.3; then exit 1; fi
if release_init test/fork '../../bad'; then exit 1; fi
release_init test/fork v1.2.3
if release_fetch '../escape' "$tmp/download"; then exit 1; fi
# Corrupt payload.
printf 'corrupt\n' > "$tmp/source/test.zip"
if release_fetch test.zip "$tmp/download"; then exit 1; fi
[[ ! -e $tmp/download/test.zip ]]
# Missing, duplicate, malformed and traversal-only entries must fail closed.
for mode in missing duplicate malformed traversal; do
    case $mode in
        missing) printf '' > "$tmp/source/SHA256SUMS" ;;
        duplicate) cp "$tmp/good-sums" "$tmp/source/SHA256SUMS"; printf '\n' >> "$tmp/source/SHA256SUMS"; while IFS= read -r line; do printf '%s\n' "$line" >> "$tmp/source/SHA256SUMS"; done < "$tmp/good-sums" ;;
        malformed) printf 'bad  test.zip\n' > "$tmp/source/SHA256SUMS" ;;
        traversal) printf '%064d  ../test.zip\n' 0 > "$tmp/source/SHA256SUMS" ;;
    esac
    if release_fetch test.zip "$tmp/download"; then echo "accepted $mode manifest" >&2; exit 1; fi
done
CURL_FAIL=true
if release_fetch test.zip "$tmp/download"; then exit 1; fi
if release_init test/fork ''; then exit 1; fi
unset CURL_FAIL

if [[ -f $here/verify-release-rootfs.sh ]]; then
    mkdir -p "$tmp/bin"
    for arch in amd64 arm64; do
        root="$tmp/rootfs/linux_$arch"
        mkdir -p "$root/app/etc" "$root/etc/ssl/certs" "$root/usr/share/zoneinfo/Asia"
        printf 'binary-%s\n' "$arch" > "$tmp/bin/ppanel-server-linux-$arch"
        cp "$tmp/bin/ppanel-server-linux-$arch" "$root/app/ppanel"
        chmod 755 "$root/app/ppanel"
        printf '%s\n' '-----BEGIN CERTIFICATE-----' > "$root/etc/ssl/certs/ca-certificates.crt"
        printf 'timezone\n' > "$root/usr/share/zoneinfo/Asia/Shanghai"
        touch "$root/app/etc/ppanel.yaml"
    done
    bash "$here/verify-release-rootfs.sh" "$tmp/rootfs" "$tmp/bin"
    : > "$tmp/rootfs/linux_arm64/etc/ssl/certs/ca-certificates.crt"
    if bash "$here/verify-release-rootfs.sh" "$tmp/rootfs" "$tmp/bin"; then exit 1; fi
fi
for script in "$here"/*.sh; do bash -n "$script"; done
printf 'PASS: production release download/checksum helpers and shell syntax\n'
