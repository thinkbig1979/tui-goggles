#!/bin/sh
# Installs the tui-capture Claude Code skill (SKILL.md + tui-goggles binary)
# from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/thinkbig1979/tui-goggles/main/install.sh | sh
#
# Environment:
#   VERSION    release tag to install (default: latest)
#   SKILL_DIR  install location (default: ~/.claude/skills/tui-capture)
#   BASE_URL   release download base (default: GitHub releases; for testing)
set -eu

repo="thinkbig1979/tui-goggles"
version="${VERSION:-latest}"
skill_dir="${SKILL_DIR:-$HOME/.claude/skills/tui-capture}"

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) echo "tui-goggles: unsupported OS $(uname -s) (Linux and macOS only)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "tui-goggles: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${BASE_URL:-}" ]; then
	base="$BASE_URL"
elif [ "$version" = latest ]; then
	base="https://github.com/$repo/releases/latest/download"
else
	base="https://github.com/$repo/releases/download/$version"
fi

download() { # url dest
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		echo "tui-goggles: need curl or wget" >&2
		exit 1
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

bin="tui-goggles_${os}_${arch}"
echo "Downloading tui-goggles ($version, $os/$arch)..."
download "$base/$bin" "$tmp/$bin"
download "$base/SKILL.md" "$tmp/SKILL.md"
download "$base/checksums.txt" "$tmp/checksums.txt"

for f in "$bin" SKILL.md; do
	want=$(awk -v f="$f" '$2 == f {print $1}' "$tmp/checksums.txt")
	got=$(sha256 "$tmp/$f")
	if [ -z "$want" ] || [ "$want" != "$got" ]; then
		echo "tui-goggles: checksum mismatch for $f" >&2
		exit 1
	fi
done

mkdir -p "$skill_dir/bin"
chmod +x "$tmp/$bin"
mv "$tmp/$bin" "$skill_dir/bin/tui-goggles"
mv "$tmp/SKILL.md" "$skill_dir/SKILL.md"
echo "Installed $("$skill_dir/bin/tui-goggles" -version) to $skill_dir"
