#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version="${CRUMBCLI_VERSION:-dev}"
if [[ ! "$version" =~ ^(dev|v[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
    echo 'CRUMBCLI_VERSION must be dev or vMAJOR.MINOR.PATCH' >&2
    exit 2
fi
repo="$PWD"
mkdir -p dist
for arch in amd64 arm64; do
    target_dir="$repo/dist/linux_$arch"
    mkdir -p "$target_dir"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$target_dir/crumbcli" ./cmd/crumbcli
    tar -czf "$repo/dist/crumbcli_${version}_linux_${arch}.tar.gz" -C "$target_dir" crumbcli -C "$repo" LICENSE README.md
done
cd dist
sha256sum ./*.tar.gz > SHA256SUMS
