#!/usr/bin/env bash
# Build/package one release target. Run from the repository root.
set -euo pipefail
: "${VERSION:?set VERSION without the leading v}"
: "${BUILD_TIME:?set BUILD_TIME to UTC RFC3339}"
export CHANNEL="${CHANNEL:-stable}"
os=${1:?goos}; arch=${2:?goarch}; out=${RELEASE_OUT:-build/release}
case "$os/$arch" in
  linux/386|linux/amd64|linux/arm64|windows/386|windows/amd64|windows/arm64|darwin/amd64|darwin/arm64) ;;
  *) echo "unsupported release target: $os/$arch" >&2; exit 1 ;;
esac
asset="ppanel-server-$os-$arch"
ext=''; [[ $os != windows ]] || ext='.exe'
mkdir -p "$out/bin" "$out/package/$asset/etc"
flags=$(VERSION="$VERSION" BUILD_TIME="$BUILD_TIME" sh script/ldflags.sh)
CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$flags" -o "$out/bin/$asset$ext" .
cp "$out/bin/$asset$ext" "$out/package/$asset/ppanel-server$ext"
cp LICENSE "$out/package/$asset/"
# Never package a locally initialized configuration containing credentials.
: > "$out/package/$asset/etc/ppanel.yaml"
cp script/install.sh "$out/package/$asset/install.sh"
if [[ $os == windows ]]; then
  (cd "$out/package" && zip -q -r "../$asset.zip" "$asset")
else
  chmod 755 "$out/package/$asset/ppanel-server"
  tar -czf "$out/$asset.tar.gz" -C "$out/package" "$asset"
fi
