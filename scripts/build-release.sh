#!/bin/bash
# Build the release binaries.
#
# Usage: scripts/build-release.sh TAG OUTDIR
#        scripts/build-release.sh v0.1.0 dist
#
# CI runs exactly this on a v* tag (.gitea/workflows/release.yml), so a
# release can be built and checked on any machine with Go before tagging.
#
# Static (CGO off), so each binary runs anywhere its OS and architecture do.
# The editor's interface is embedded from internal/editor/ui/dist, which is
# committed and checked against its source on every pull request. The tag is
# stamped in, so `hotdog-cms version` and the release can't disagree.
set -euo pipefail

TAG="${1:?usage: build-release.sh TAG OUTDIR}"
OUT="${2:?usage: build-release.sh TAG OUTDIR}"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] \
  || { echo "tag $TAG is not v<major>.<minor>.<patch>[-<pre-release>]" >&2; exit 1; }
[ -f internal/editor/ui/dist/index.html ] \
  || { echo "internal/editor/ui/dist is missing: npm ci && npm run build in editor-ui/" >&2; exit 1; }

mkdir -p "$OUT"
files=()
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os=${target%/*} arch=${target#*/}
  name="hotdog-cms-$os-$arch"
  [ "$os" = windows ] && name="$name.exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about.Version=${TAG#v}" \
    -o "$OUT/$name" ./cmd/hotdog-cms
  files+=("$name")
done
(cd "$OUT" && sha256sum "${files[@]}" > SHA256SUMS)
cat "$OUT/SHA256SUMS"
