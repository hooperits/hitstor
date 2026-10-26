# wizard-server.sh — Interactive server setup wizard for HitStor.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# Wizard variables (set by wizard or --no-wizard env vars).
DATA_DIR=""
DASH_PASS=""
GRPC_PORT=""
DASH_PORT=""
ADVERTISE_ADDR=""

# wizard_server — runs the interactive server configuration wizard.
wizard_server() {
    color_print bold "Server Setup Wizard"
    echo ""

    # 1. Data directory.
    prompt_value DATA_DIR "Data directory" "/var/lib/hitstor"
    show_disk_space "$DATA_DIR"
    echo ""

    # 2. Dashboard password (optional).
    echo "  Dashboard password protects the web UI."
    echo "  Leave empty for open access (no login required)."
    prompt_password DASH_PASS "Dashboard password" 0
    echo ""

    # 3. gRPC listen port.
    while true; do
        prompt_value GRPC_PORT "gRPC listen port" "8420"
        if validate_port "$GRPC_PORT"; then
            break
        fi
        color_print red "    Invalid port number (must be 1-65535)"
    done

    # 4. Dashboard listen port.
    while true; do
        prompt_value DASH_PORT "Dashboard listen port" "8421"
        if validate_port "$DASH_PORT"; then
            break
        fi
        color_print red "    Invalid port number (must be 1-65535)"
    done

    # 5. Advertise address (IP agents use to reach this server).
    _default_ip=$(detect_outbound_ip)
    echo "  This address is sent to agents so they know how to reach the server."
    prompt_value ADVERTISE_ADDR "Server advertise address" "${_default_ip}:${GRPC_PORT}"
    echo ""

    # Summary.
    echo ""
    color_print bold "Configuration Summary"
    echo "  Data directory:      $DATA_DIR"
    if [ -n "$DASH_PASS" ]; then
        echo "  Dashboard password:  ********"
    else
        echo "  Dashboard password:  (open access)"
    fi
    echo "  gRPC port:           $GRPC_PORT"
    echo "  Dashboard port:      $DASH_PORT"
    echo "  Advertise address:   $ADVERTISE_ADDR"
    echo ""

    if ! prompt_yesno "Proceed with installation?" "y"; then
        echo "Installation cancelled."
        exit 0
    fi
}

# nowizard_server — reads config from environment variables (--no-wizard mode).
nowizard_server() {
    DATA_DIR="${HITSTOR_DATA_DIR:-/var/lib/hitstor}"
    DASH_PASS="${HITSTOR_DASHBOARD_PASSWORD:-}"
    GRPC_PORT="${HITSTOR_GRPC_PORT:-8420}"
    DASH_PORT="${HITSTOR_DASHBOARD_PORT:-8421}"
    ADVERTISE_ADDR="${HITSTOR_ADVERTISE_ADDR:-}"

    if ! validate_port "$GRPC_PORT"; then
        color_print red "Error: invalid HITSTOR_GRPC_PORT: $GRPC_PORT"
        exit 1
    fi

    if ! validate_port "$DASH_PORT"; then
        color_print red "Error: invalid HITSTOR_DASHBOARD_PORT: $DASH_PORT"
        exit 1
    fi

    # Auto-detect advertise address if not provided.
    if [ -z "$ADVERTISE_ADDR" ]; then
        _default_ip=$(detect_outbound_ip)
        ADVERTISE_ADDR="${_default_ip}:${GRPC_PORT}"
    fi

    echo "  Data directory:      $DATA_DIR"
    echo "  gRPC port:           $GRPC_PORT"
    echo "  Dashboard port:      $DASH_PORT"
    echo "  Advertise address:   $ADVERTISE_ADDR"
    echo ""
}

# install_server_files — creates user, dirs, config, systemd.
install_server_files() {
    # Create system user and group.
    if ! getent group hitstor >/dev/null 2>&1; then
        groupadd --system hitstor
    fi
    if ! getent passwd hitstor >/dev/null 2>&1; then
        useradd --system --gid hitstor --home-dir /var/lib/hitstor \
                --shell /usr/sbin/nologin --no-create-home hitstor
    fi
    grant_hitstor_journal_access
    color_print green "[+] System user 'hitstor' ready"

    # Create directories.
    mkdir -p "$DATA_DIR" /var/log/hitstor /etc/hitstor
    chown hitstor:hitstor "$DATA_DIR" /var/log/hitstor /etc/hitstor
    chmod 750 "$DATA_DIR" /var/log/hitstor /etc/hitstor
    color_print green "[+] Directories created"

    # Install binary.
    install -m 755 "$TMPDIR/hitstor" /usr/bin/hitstor
    color_print green "[+] Installed binary to /usr/bin/hitstor"

    # Install systemd service.
    install -m 644 "$TMPDIR/hitstor-server.service" /usr/lib/systemd/system/
    systemctl daemon-reload
    color_print green "[+] Systemd service installed"

    # Install config examples (always overwrite on upgrade).
    install -m 644 "$TMPDIR/config.yaml.example" /etc/hitstor/
    install -m 644 "$TMPDIR/server.env.example" /etc/hitstor/

    # Write config.yaml with wizard values.
    _config="/etc/hitstor/config.yaml"
    cat > "$_config" <<CFGEOF
serve_port: ${GRPC_PORT}
serve_advertise_addr: "${ADVERTISE_ADDR}"
serve_dashboard_port: ${DASH_PORT}
CFGEOF
    if [ -n "$DASH_PASS" ]; then
        printf "serve_dashboard_password: \"%s\"\n" "$DASH_PASS" >> "$_config"
    fi
    chown hitstor:hitstor "$_config"
    chmod 600 "$_config"

    # Write empty server.env (repositories are configured via the dashboard).
    _env="/etc/hitstor/server.env"
    printf "# Repository passwords are managed via the dashboard.\n" > "$_env"
    chown hitstor:hitstor "$_env"
    chmod 600 "$_env"
    color_print green "[+] Configuration files written"

    # Enable and start service.
    systemctl enable --now hitstor-server
    color_print green "[+] Service enabled and started"
}

# print_server_success — prints the success summary.
print_server_success() {
    echo ""
    color_print bold "============================================="
    color_print green "  Installation complete!"
    color_print bold "============================================="
    echo ""
    echo "  Dashboard: https://localhost:${DASH_PORT}"
    echo "  gRPC port: ${GRPC_PORT}"
    echo ""
    echo "  Open the dashboard to create your first repository"
    echo "  and start configuring backup agents."
    echo ""
}
