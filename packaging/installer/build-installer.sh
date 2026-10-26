#!/bin/sh
# Build a self-extracting .run installer for HitStor.
# Usage: build-installer.sh server|agent <version> <arch>
set -e

TYPE="$1"
VERSION="$2"
ARCH="$3"

if [ -z "$TYPE" ] || [ -z "$VERSION" ] || [ -z "$ARCH" ]; then
    echo "Usage: $0 server|agent <version> <arch>" >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# inline_libs SRC DEST — replace __INLINE_*__ markers with library contents.
inline_libs() {
    _src="$1"
    _dest="$2"
    cp "$_src" "$_dest"
    for _marker in COMMON WIZARD_SERVER WIZARD_AGENT VERIFY UNINSTALL UPGRADE; do
        _libname=$(echo "$_marker" | tr 'A-Z' 'a-z' | tr '_' '-')
        _libfile="$SCRIPT_DIR/lib/${_libname}.sh"
        if [ -f "$_libfile" ] && grep -q "__INLINE_${_marker}__" "$_dest"; then
            sed "/__INLINE_${_marker}__/r $_libfile" "$_dest" | \
                sed "/__INLINE_${_marker}__/d" > "${_dest}.tmp"
            mv "${_dest}.tmp" "$_dest"
        fi
    done
}

PAYLOAD_DIR=$(mktemp -d)
trap 'rm -rf "$PAYLOAD_DIR"' EXIT

case "$TYPE" in
    server)
        BINARY="$PROJECT_DIR/bin/hitstor"
        if [ "$ARCH" = "arm64" ]; then
            BINARY="$PROJECT_DIR/bin/hitstor-arm64"
        fi
        cp "$BINARY" "$PAYLOAD_DIR/hitstor"
        cp "$SCRIPT_DIR/../systemd/hitstor-server.service" "$PAYLOAD_DIR/"
        cp "$SCRIPT_DIR/../config/server-config.yaml.example" "$PAYLOAD_DIR/config.yaml.example"
        cp "$SCRIPT_DIR/../systemd/hitstor-server.env.example" "$PAYLOAD_DIR/server.env.example"
        HEADER="$SCRIPT_DIR/server-installer-v2.sh.in"
        OUTPUT="$PROJECT_DIR/dist/hitstor-server-${VERSION}-linux-${ARCH}.run"
        ;;
    agent)
        BINARY="$PROJECT_DIR/bin/hitstor-agent"
        if [ "$ARCH" = "arm64" ]; then
            BINARY="$PROJECT_DIR/bin/hitstor-agent-arm64"
        fi
        cp "$BINARY" "$PAYLOAD_DIR/hitstor-agent"
        cp "$SCRIPT_DIR/../systemd/hitstor-agent.service" "$PAYLOAD_DIR/"
        cp "$SCRIPT_DIR/../systemd/hitstor-agent.timer" "$PAYLOAD_DIR/"
        cp "$SCRIPT_DIR/../config/agent-config.yaml.example" "$PAYLOAD_DIR/agent-config.yaml.example"
        cp "$SCRIPT_DIR/../systemd/hitstor-agent.env.example" "$PAYLOAD_DIR/agent.env.example"
        HEADER="$SCRIPT_DIR/agent-installer-v2.sh.in"
        OUTPUT="$PROJECT_DIR/dist/hitstor-agent-${VERSION}-linux-${ARCH}.run"
        ;;
    *)
        echo "Error: type must be 'server' or 'agent'" >&2
        exit 1
        ;;
esac

if [ ! -f "$BINARY" ]; then
    echo "Error: binary not found: $BINARY" >&2
    echo "Run 'make build' or 'make agent' first." >&2
    exit 1
fi

# Create tar.gz payload (outside PAYLOAD_DIR to avoid "file changed" error).
PAYLOAD_TAR=$(mktemp)
tar czf "$PAYLOAD_TAR" -C "$PAYLOAD_DIR" .

# Assemble: inline libraries, substitute version, append payload.
mkdir -p "$(dirname "$OUTPUT")"
ASSEMBLED=$(mktemp)
inline_libs "$HEADER" "$ASSEMBLED"
sed "s/__VERSION__/$VERSION/g" "$ASSEMBLED" > "$OUTPUT"
rm -f "$ASSEMBLED"
cat "$PAYLOAD_TAR" >> "$OUTPUT"
rm -f "$PAYLOAD_TAR"
chmod +x "$OUTPUT"

SIZE=$(du -h "$OUTPUT" | cut -f1)
echo "Built: $OUTPUT ($SIZE)"
