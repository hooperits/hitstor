#!/bin/sh
set -e

# Create config and plugin directories.
mkdir -p /etc/hitstor /usr/lib/hitstor/plugins
chmod 755 /usr/lib/hitstor/plugins 2>/dev/null || true

# Copy example config files if no existing config.
if [ ! -f /etc/hitstor/agent.env ]; then
    cp /etc/hitstor/agent.env.example /etc/hitstor/agent.env
    chmod 600 /etc/hitstor/agent.env
fi

if [ ! -f /etc/hitstor/agent-config.yaml ]; then
    cp /etc/hitstor/agent-config.yaml.example /etc/hitstor/agent-config.yaml
    chmod 600 /etc/hitstor/agent-config.yaml
fi

# Reload systemd to pick up new service/timer files.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
fi

echo ""
echo "HitStor agent installed successfully."
echo ""
echo "Next steps:"
echo "  1. Edit configuration:       sudo nano /etc/hitstor/agent.env"
echo "  2. Start daemon mode:        sudo systemctl enable --now hitstor-agent"
echo "     Or use timer mode:        sudo systemctl enable --now hitstor-agent.timer"
echo ""
