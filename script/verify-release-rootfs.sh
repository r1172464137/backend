#!/usr/bin/env bash
# Inspect buildx's platform-split rootfs exports, without executing either binary.
set -euo pipefail
root=${1:?rootfs export directory}; binaries=${2:?release binary directory}
for arch in amd64 arm64; do
    dir="$root/linux_$arch"
    test -s "$dir/app/ppanel"
    test -x "$dir/app/ppanel"
    cmp "$binaries/ppanel-server-linux-$arch" "$dir/app/ppanel"
    test -s "$dir/etc/ssl/certs/ca-certificates.crt"
    grep -q -- '-----BEGIN CERTIFICATE-----' "$dir/etc/ssl/certs/ca-certificates.crt"
    test -s "$dir/usr/share/zoneinfo/Asia/Shanghai"
    test -f "$dir/app/etc/ppanel.yaml"
    test ! -s "$dir/app/etc/ppanel.yaml"
done
