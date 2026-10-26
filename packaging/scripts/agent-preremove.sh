#!/bin/sh
set -e

# Stop and disable the service and timer before removal.
if command -v systemctl >/dev/null 2>&1; then
    systemctl stop hitstor-agent 2>/dev/null || true
    systemctl disable hitstor-agent 2>/dev/null || true
    systemctl stop hitstor-agent.timer 2>/dev/null || true
    systemctl disable hitstor-agent.timer 2>/dev/null || true
fi
