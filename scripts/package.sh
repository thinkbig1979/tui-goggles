#!/usr/bin/env bash
# Builds release artifacts for every supported platform.
#
#   scripts/package.sh VERSION [OUTDIR]
#
# For each OS/arch it writes:
#   tui-goggles_<os>_<arch>               the binary
#   tui-capture-skill_<os>_<arch>.zip     the Claude Code skill (SKILL.md + bin/)
# plus SKILL.md and checksums.txt (sha256 of every file).
set -euo pipefail

version=${1:?usage: scripts/package.sh VERSION [OUTDIR]}
out=${2:-dist}
cd "$(dirname "$0")/.."

platforms="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64"

rm -rf "$out"
mkdir -p "$out"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

for p in $platforms; do
	os=${p%/*}
	arch=${p#*/}
	bin="$out/tui-goggles_${os}_${arch}"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$bin" ./cmd/tui-goggles

	skill="$stage/$os-$arch/tui-capture"
	mkdir -p "$skill/bin"
	cp SKILL.md "$skill/SKILL.md"
	cp "$bin" "$skill/bin/tui-goggles"
	(cd "$stage/$os-$arch" && zip -qr - tui-capture) > "$out/tui-capture-skill_${os}_${arch}.zip"
done

cp SKILL.md "$out/SKILL.md"
(cd "$out" && sha256sum -- * > checksums.txt)
echo "Built $version into $out:"
ls -1 "$out"
