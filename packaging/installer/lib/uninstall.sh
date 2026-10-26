# uninstall.sh — Uninstall functions for HitStor server and agent.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# uninstall_server — removes all HitStor server components.
uninstall_server() {
    color_print bold "HitStor Server Uninstaller"
    echo ""

    if [ ! -f /usr/bin/hitstor ] && ! systemctl list-unit-files hitstor-server.service >/dev/null 2>&1; then
        color_print yellow "HitStor server does not appear to be installed."
        exit 0
    fi

    echo "  The following will be removed:"
    echo "    - Binary:         /usr/bin/hitstor"
    echo "    - Service:        hitstor-server.service"
    echo "    - Configuration:  /etc/hitstor/"
    echo "    - Logs:           /var/log/hitstor/"
    echo "    - System user:    hitstor"
    echo ""

    if ! prompt_yesno "Proceed with uninstall?" "n"; then
        echo "Uninstall cancelled."
        exit 0
    fi

    echo ""

    # Stop and disable service.
    if systemctl is-active --quiet hitstor-server 2>/dev/null; then
        systemctl stop hitstor-server
        color_print green "[+] Service stopped"
    fi
    if systemctl is-enabled --quiet hitstor-server 2>/dev/null; then
        systemctl disable hitstor-server 2>/dev/null
        color_print green "[+] Service disabled"
    fi

    # Remove binary.
    rm -f /usr/bin/hitstor
    rm -f /usr/bin/hitstor.rollback
    color_print green "[+] Binary removed"

    # Remove systemd unit.
    rm -f /usr/lib/systemd/system/hitstor-server.service
    systemctl daemon-reload
    color_print green "[+] Systemd service removed"

    # Collect repository paths from config before removing it.
    _repo_paths=""
    if [ -f /etc/hitstor/config.yaml ]; then
        _repo_paths=$(grep '^ *path:' /etc/hitstor/config.yaml 2>/dev/null | sed 's/^ *path: *//' | tr -d '"' | tr -d "'")
    fi

    # Remove config files.
    rm -rf /etc/hitstor
    color_print green "[+] Configuration removed"

    # Remove log directory.
    rm -rf /var/log/hitstor
    color_print green "[+] Logs removed"

    # Ask about repository data directories.
    _found_data=0
    for _rpath in $_repo_paths; do
        if [ -d "$_rpath" ]; then
            _found_data=1
        fi
    done
    # Also check the default data directory.
    if [ -d /var/lib/hitstor ]; then
        _found_data=1
    fi

    if [ "$_found_data" = "1" ]; then
        echo ""
        color_print bold "Repository data found:"
        for _rpath in $_repo_paths; do
            if [ -d "$_rpath" ]; then
                echo "    $_rpath"
            fi
        done
        if [ -d /var/lib/hitstor ]; then
            # Show default dir if not already listed.
            _already_listed=0
            for _rpath in $_repo_paths; do
                if [ "$_rpath" = "/var/lib/hitstor" ]; then
                    _already_listed=1
                fi
            done
            if [ "$_already_listed" = "0" ]; then
                echo "    /var/lib/hitstor"
            fi
        fi
        echo ""
        color_print red "WARNING: Removing repository data will permanently destroy all backups!"
        echo ""
        if prompt_yesno "Remove all repository data?" "n"; then
            for _rpath in $_repo_paths; do
                if [ -d "$_rpath" ]; then
                    rm -rf "$_rpath"
                fi
            done
            if [ -d /var/lib/hitstor ]; then
                rm -rf /var/lib/hitstor
            fi
            color_print green "[+] Repository data removed"
        else
            color_print yellow "[*] Repository data preserved"
            echo "    After reinstalling, add the same path in the dashboard"
            echo "    to reconnect to your existing repository."
        fi
    fi

    # Remove system user and group.
    if getent passwd hitstor >/dev/null 2>&1; then
        userdel hitstor 2>/dev/null || true
    fi
    if getent group hitstor >/dev/null 2>&1; then
        groupdel hitstor 2>/dev/null || true
    fi
    color_print green "[+] System user removed"

    echo ""
    color_print green "Uninstall complete."
    echo ""
}

# uninstall_agent — removes all HitStor agent components.
uninstall_agent() {
    color_print bold "HitStor Agent Uninstaller"
    echo ""

    if [ ! -f /usr/bin/hitstor-agent ] && ! systemctl list-unit-files hitstor-agent.service >/dev/null 2>&1; then
        color_print yellow "HitStor agent does not appear to be installed."
        exit 0
    fi

    echo "  The following will be removed:"
    echo "    - Binary:         /usr/bin/hitstor-agent"
    echo "    - Service:        hitstor-agent.service"
    echo "    - Timer:          hitstor-agent.timer"
    echo "    - Configuration:  /etc/hitstor/"
    echo ""

    if ! prompt_yesno "Proceed with uninstall?" "n"; then
        echo "Uninstall cancelled."
        exit 0
    fi

    echo ""

    # Stop and disable service and timer.
    if systemctl is-active --quiet hitstor-agent 2>/dev/null; then
        systemctl stop hitstor-agent
        color_print green "[+] Service stopped"
    fi
    if systemctl is-enabled --quiet hitstor-agent 2>/dev/null; then
        systemctl disable hitstor-agent 2>/dev/null
        color_print green "[+] Service disabled"
    fi
    if systemctl is-active --quiet hitstor-agent.timer 2>/dev/null; then
        systemctl stop hitstor-agent.timer
    fi
    if systemctl is-enabled --quiet hitstor-agent.timer 2>/dev/null; then
        systemctl disable hitstor-agent.timer 2>/dev/null
        color_print green "[+] Timer disabled"
    fi

    # Remove binary.
    rm -f /usr/bin/hitstor-agent
    rm -f /usr/bin/hitstor-agent.rollback
    color_print green "[+] Binary removed"

    # Remove systemd units.
    rm -f /usr/lib/systemd/system/hitstor-agent.service
    rm -f /usr/lib/systemd/system/hitstor-agent.timer
    systemctl daemon-reload
    color_print green "[+] Systemd units removed"

    # Remove config files.
    rm -rf /etc/hitstor
    color_print green "[+] Configuration removed"

    echo ""
    color_print green "Uninstall complete."
    echo ""
}
