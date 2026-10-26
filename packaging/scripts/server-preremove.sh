#!/bin/sh
set -e

# Stop and disable the service before removal.
if command -v systemctl >/dev/null 2>&1; then
    systemctl stop hitstor-server 2>/dev/null || true
    systemctl disable hitstor-server 2>/dev/null || true
fi
