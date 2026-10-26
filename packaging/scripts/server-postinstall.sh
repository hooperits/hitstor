#!/bin/sh
set -e

# Create system user and group if they don't exist.
if ! getent group hitstor >/dev/null 2>&1; then
    groupadd --system hitstor
fi
if ! getent passwd hitstor >/dev/null 2>&1; then
    useradd --system --gid hitstor --home-dir /var/lib/hitstor \
            --shell /usr/sbin/nologin --no-create-home hitstor
fi

# View Logs reads journald; grant read access to the service user.
if [ -f /usr/lib/hitstor/grant-journal-access.sh ]; then
    sh /usr/lib/hitstor/grant-journal-access.sh
else
    if getent group systemd-journal >/dev/null 2>&1; then
        if ! id -nG hitstor 2>/dev/null | tr ' ' '\n' | grep -qx systemd-journal; then
            usermod -aG systemd-journal hitstor 2>/dev/null || true
        fi
    fi
fi

# Create required directories.
mkdir -p /var/lib/hitstor /var/log/hitstor /etc/hitstor /usr/lib/hitstor/plugins
chmod 755 /usr/lib/hitstor/plugins 2>/dev/null || true
chown hitstor:hitstor /var/lib/hitstor /var/log/hitstor
chmod 750 /var/lib/hitstor
chmod 750 /var/log/hitstor

# Copy example config files if no existing config.
if [ ! -f /etc/hitstor/server.env ]; then
    cp /etc/hitstor/server.env.example /etc/hitstor/server.env
    chmod 600 /etc/hitstor/server.env
    chown hitstor:hitstor /etc/hitstor/server.env
fi

if [ ! -f /etc/hitstor/config.yaml ]; then
    cp /etc/hitstor/config.yaml.example /etc/hitstor/config.yaml
    chmod 600 /etc/hitstor/config.yaml
    chown hitstor:hitstor /etc/hitstor/config.yaml
fi

# Reload systemd to pick up new service file.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
fi

echo ""
echo "HitStor server installed successfully."
echo ""
echo "Next steps:"
echo "  1. Set the repository password in /etc/hitstor/server.env"
echo "  2. Initialize a repository:  sudo -u hitstor HITSTOR_PASSWORD=<pass> hitstor init /var/lib/hitstor/repo"
echo "  3. Start the service:        sudo systemctl enable --now hitstor-server"
echo "  4. Open the dashboard:       http://localhost:8421"
echo ""
