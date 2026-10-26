#!/usr/bin/env bash
# install-agent.sh — download and install hitstor-agent from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/hooperits/hitstor/master/scripts/install-agent.sh | sudo bash
#   curl -fsSL ... | sudo bash -s -- --version v1.0.0
#   curl -fsSL ... | sudo bash -s -- --version latest --arch arm64
#
set -euo pipefail

REPO="hooperits/hitstor"
VERSION="${HITSTOR_AGENT_VERSION:-latest}"
ARCH=""
RUN_WIZARD=true

usage() {
    cat <<EOF
Usage: install-agent.sh [options]

Options:
  --version VER   Release tag (default: latest)
  --arch ARCH     amd64 or arm64 (default: auto-detect)
  --no-wizard     Skip the interactive registration wizard
  -h, --help      Show this help
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --arch) ARCH="$2"; shift 2 ;;
        --no-wizard) RUN_WIZARD=false; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: run as root (sudo)." >&2
    exit 1
fi

if [ -z "$ARCH" ]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) echo "Error: unsupported architecture $(uname -m)" >&2; exit 1 ;;
    esac
fi

if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+' || true)
    if [ -z "$VERSION" ]; then
        echo "Error: could not resolve latest release. Set --version explicitly." >&2
        exit 1
    fi
fi

ASSET="hitstor-agent-${VERSION#v}-linux-${ARCH}.run"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL}..."
if ! curl -fsSL -o "${TMP}/${ASSET}" "$URL"; then
    echo "Error: download failed. Check version and architecture." >&2
    exit 1
fi

chmod +x "${TMP}/${ASSET}"

if [ "$RUN_WIZARD" = true ]; then
    "${TMP}/${ASSET}"
else
    "${TMP}/${ASSET}" --no-wizard
fi

echo "hitstor-agent installed. Register the agent from the HitStor dashboard."