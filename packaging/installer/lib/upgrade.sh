# upgrade.sh — Upgrade and rollback functions for HitStor installers.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# _get_version BINARY — prints the current version of the installed binary.
_get_version() {
    _bin="$1"
    if [ -x "$_bin" ]; then
        _ver=$("$_bin" version --json 2>/dev/null | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
        if [ -n "$_ver" ]; then
            echo "$_ver"
            return
        fi
    fi
    echo "unknown"
}

# upgrade_server — upgrades an existing HitStor server installation.
upgrade_server() {
    _current_ver=$(_get_version /usr/bin/hitstor)
    echo ""
    color_print bold "Existing installation detected"
    echo "  Current version: v${_current_ver}"
    echo "  New version:     v${HITSTOR_VERSION}"
    echo ""

    if [ "$_current_ver" = "$HITSTOR_VERSION" ]; then
        color_print yellow "  Same version is already installed."
    fi

    # Ask for confirmation (skip in --no-wizard mode).
    if [ "$FLAG_NO_WIZARD" -eq 0 ]; then
        if ! prompt_yesno "Proceed with upgrade?" "y"; then
            echo "Upgrade cancelled."
            exit 0
        fi
    fi

    echo ""

    # Record current service state.
    _was_running=0
    _was_enabled=0
    if systemctl is-active --quiet hitstor-server 2>/dev/null; then
        _was_running=1
    fi
    if systemctl is-enabled --quiet hitstor-server 2>/dev/null; then
        _was_enabled=1
    fi

    # Backup current binary.
    cp /usr/bin/hitstor /usr/bin/hitstor.rollback
    color_print green "[+] Binary backed up to /usr/bin/hitstor.rollback"

    # Stop service.
    if [ "$_was_running" -eq 1 ]; then
        systemctl stop hitstor-server
        color_print green "[+] Service stopped"
    fi

    # Install new binary.
    install -m 755 "$TMPDIR/hitstor" /usr/bin/hitstor
    color_print green "[+] New binary installed"

    grant_hitstor_journal_access

    # Update systemd unit.
    install -m 644 "$TMPDIR/hitstor-server.service" /usr/lib/systemd/system/
    systemctl daemon-reload
    color_print green "[+] Systemd service updated"

    # Update example configs only (never touch working configs).
    install -m 644 "$TMPDIR/config.yaml.example" /etc/hitstor/ 2>/dev/null || true
    install -m 644 "$TMPDIR/server.env.example" /etc/hitstor/ 2>/dev/null || true
    color_print green "[+] Example configs updated"

    # Restart service if it was running.
    if [ "$_was_running" -eq 1 ] || [ "$_was_enabled" -eq 1 ]; then
        systemctl start hitstor-server

        # Read ports from existing config for verification.
        _grpc_port=8420
        _dash_port=8421
        if [ -f /etc/hitstor/config.yaml ]; then
            _p=$(sed -n 's/^serve_port: *\([0-9]*\)/\1/p' /etc/hitstor/config.yaml)
            [ -n "$_p" ] && _grpc_port="$_p"
            _p=$(sed -n 's/^serve_dashboard_port: *\([0-9]*\)/\1/p' /etc/hitstor/config.yaml)
            [ -n "$_p" ] && _dash_port="$_p"
        fi

        # Verify.
        if wait_and_verify_server "$_grpc_port" "$_dash_port"; then
            rm -f /usr/bin/hitstor.rollback
            echo ""
            color_print green "Upgrade to v${HITSTOR_VERSION} complete!"
            echo ""
        else
            # Rollback.
            echo ""
            color_print red "Verification failed — rolling back to v${_current_ver}..."
            systemctl stop hitstor-server 2>/dev/null || true
            cp /usr/bin/hitstor.rollback /usr/bin/hitstor
            rm -f /usr/bin/hitstor.rollback
            systemctl start hitstor-server
            color_print yellow "Rolled back to v${_current_ver}. The previous version is running."
            color_print yellow "Check logs: sudo journalctl -u hitstor-server -f"
            exit 1
        fi
    else
        rm -f /usr/bin/hitstor.rollback
        echo ""
        color_print green "Upgrade to v${HITSTOR_VERSION} complete!"
        color_print yellow "Service was not running. Start it with: sudo systemctl start hitstor-server"
        echo ""
    fi
}

# upgrade_agent — upgrades an existing HitStor agent installation.
upgrade_agent() {
    _current_ver=$(_get_version /usr/bin/hitstor-agent)
    echo ""
    color_print bold "Existing installation detected"
    echo "  Current version: v${_current_ver}"
    echo "  New version:     v${HITSTOR_VERSION}"
    echo ""

    if [ "$_current_ver" = "$HITSTOR_VERSION" ]; then
        color_print yellow "  Same version is already installed."
    fi

    # Ask for confirmation (skip in --no-wizard mode).
    if [ "$FLAG_NO_WIZARD" -eq 0 ]; then
        if ! prompt_yesno "Proceed with upgrade?" "y"; then
            echo "Upgrade cancelled."
            exit 0
        fi
    fi

    echo ""

    # Record current service state.
    _was_running=0
    _was_enabled=0
    if systemctl is-active --quiet hitstor-agent 2>/dev/null; then
        _was_running=1
    fi
    if systemctl is-enabled --quiet hitstor-agent 2>/dev/null; then
        _was_enabled=1
    fi

    # Backup current binary.
    cp /usr/bin/hitstor-agent /usr/bin/hitstor-agent.rollback
    color_print green "[+] Binary backed up to /usr/bin/hitstor-agent.rollback"

    # Stop service.
    if [ "$_was_running" -eq 1 ]; then
        systemctl stop hitstor-agent
        color_print green "[+] Service stopped"
    fi

    # Install new binary.
    install -m 755 "$TMPDIR/hitstor-agent" /usr/bin/hitstor-agent
    color_print green "[+] New binary installed"

    # Update systemd units.
    install -m 644 "$TMPDIR/hitstor-agent.service" /usr/lib/systemd/system/
    install -m 644 "$TMPDIR/hitstor-agent.timer" /usr/lib/systemd/system/
    systemctl daemon-reload
    color_print green "[+] Systemd units updated"

    # Update example configs only.
    install -m 644 "$TMPDIR/agent-config.yaml.example" /etc/hitstor/ 2>/dev/null || true
    install -m 644 "$TMPDIR/agent.env.example" /etc/hitstor/ 2>/dev/null || true
    color_print green "[+] Example configs updated"

    # Restart service if it was running.
    if [ "$_was_running" -eq 1 ] || [ "$_was_enabled" -eq 1 ]; then
        systemctl start hitstor-agent

        # Verify.
        if wait_and_verify_agent; then
            rm -f /usr/bin/hitstor-agent.rollback
            echo ""
            color_print green "Upgrade to v${HITSTOR_VERSION} complete!"
            echo ""
        else
            # Rollback.
            echo ""
            color_print red "Verification failed — rolling back to v${_current_ver}..."
            systemctl stop hitstor-agent 2>/dev/null || true
            cp /usr/bin/hitstor-agent.rollback /usr/bin/hitstor-agent
            rm -f /usr/bin/hitstor-agent.rollback
            systemctl start hitstor-agent
            color_print yellow "Rolled back to v${_current_ver}. The previous version is running."
            color_print yellow "Check logs: sudo journalctl -u hitstor-agent -f"
            exit 1
        fi
    else
        rm -f /usr/bin/hitstor-agent.rollback
        echo ""
        color_print green "Upgrade to v${HITSTOR_VERSION} complete!"
        color_print yellow "Service was not running. Start it with: sudo systemctl start hitstor-agent"
        echo ""
    fi
}
