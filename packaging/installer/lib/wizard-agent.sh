# wizard-agent.sh — Interactive agent setup wizard for HitStor.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# Wizard variables (set by wizard or --no-wizard env vars).
LISTEN_PORT="8422"

# wizard_agent — runs the interactive agent configuration wizard.
wizard_agent() {
    color_print bold "Agent Setup Wizard"
    echo ""
    echo "  The agent will be managed from the HitStor dashboard."
    echo "  After installation, add this agent from the dashboard"
    echo "  using its IP address and token."
    echo ""

    # Listen port.
    prompt_value LISTEN_PORT "Agent listen port" "8422"
    echo ""

    # Summary.
    color_print bold "Configuration Summary"
    echo "  Setup mode:  Dashboard-managed"
    echo "  Listen port: $LISTEN_PORT"
    echo ""

    if ! prompt_yesno "Proceed with installation?" "y"; then
        echo "Installation cancelled."
        exit 0
    fi
}

# nowizard_agent — reads config from environment variables (--no-wizard mode).
nowizard_agent() {
    LISTEN_PORT="${HITSTOR_AGENT_LISTEN_PORT:-8422}"

    echo "  Setup mode:  Dashboard-managed"
    echo "  Listen port: $LISTEN_PORT"
    echo ""
}

# install_agent_files — creates config directory, writes config, installs service.
install_agent_files() {
    # Create config directory.
    mkdir -p /etc/hitstor
    color_print green "[+] Configuration directory ready"

    # Install binary.
    install -m 755 "$TMPDIR/hitstor-agent" /usr/bin/hitstor-agent
    color_print green "[+] Installed binary to /usr/bin/hitstor-agent"

    # Install systemd service and timer.
    install -m 644 "$TMPDIR/hitstor-agent.service" /usr/lib/systemd/system/
    install -m 644 "$TMPDIR/hitstor-agent.timer" /usr/lib/systemd/system/
    systemctl daemon-reload
    color_print green "[+] Systemd service and timer installed"

    # Install config examples (always overwrite on upgrade).
    install -m 644 "$TMPDIR/agent-config.yaml.example" /etc/hitstor/
    install -m 644 "$TMPDIR/agent.env.example" /etc/hitstor/

    # Write agent.env with listen port (preserve credentials on upgrade).
    _env="/etc/hitstor/agent.env"
    if [ -f "$_env" ] && grep -q "HITSTOR_AGENT_SERVER=" "$_env" 2>/dev/null; then
        # Upgrade: agent is registered — preserve server/key/cert, update listen port.
        if grep -q "HITSTOR_AGENT_LISTEN_PORT=" "$_env"; then
            sed -i "s/^HITSTOR_AGENT_LISTEN_PORT=.*/HITSTOR_AGENT_LISTEN_PORT=${LISTEN_PORT}/" "$_env"
        else
            echo "HITSTOR_AGENT_LISTEN_PORT=${LISTEN_PORT}" >> "$_env"
        fi
        color_print green "[+] Agent environment file updated (credentials preserved)"
    else
        # Fresh install or unregistered — write new file.
        cat > "$_env" <<ENVEOF
HITSTOR_AGENT_LISTEN_PORT=${LISTEN_PORT}
ENVEOF
        chmod 600 "$_env"
        color_print green "[+] Agent environment file written"
    fi

    # Write agent-config.yaml — paths configured from dashboard.
    _config="/etc/hitstor/agent-config.yaml"
    printf "# Backup paths are configured from the dashboard.\nagent_paths: []\n" > "$_config"
    chmod 600 "$_config"
    color_print green "[+] Agent configuration file written"

    # Enable and start agent service.
    systemctl enable --now hitstor-agent
    color_print green "[+] Agent service enabled and started"
}

# print_agent_success — prints the success summary.
print_agent_success() {
    echo ""
    color_print bold "============================================="
    color_print green "  Installation complete!"
    color_print bold "============================================="
    echo ""

    # Retrieve token (daemon generates it on first start).
    AGENT_TOKEN=$(/usr/bin/hitstor-agent show-id 2>/dev/null || true)
    if [ -z "$AGENT_TOKEN" ]; then
        # Wait briefly for daemon to generate it.
        sleep 2
        AGENT_TOKEN=$(/usr/bin/hitstor-agent show-id 2>/dev/null || true)
    fi

    echo "  Agent is listening on port: $LISTEN_PORT"
    echo "  Agent is waiting for registration from the dashboard."
    echo ""
    color_print bold "  Agent Token:"
    if [ -n "$AGENT_TOKEN" ]; then
        echo "  $AGENT_TOKEN"
    else
        echo "  (run 'sudo hitstor-agent show-id' to display)"
    fi
    echo ""
    color_print bold "  Next steps:"
    echo "    1. Open the HitStor dashboard on the server"
    echo "    2. Go to Agents > + Add Agent"
    echo "    3. Enter this machine's IP, port $LISTEN_PORT, and the token above"
    echo "    4. Create backup jobs from the Backups section"
    echo ""

    echo "  Useful commands:"
    echo "    sudo systemctl status hitstor-agent"
    echo "    sudo journalctl -u hitstor-agent -f"
    echo "    sudo hitstor-agent show-id"
    echo ""
}
