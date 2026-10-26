# verify.sh — Post-install verification functions for HitStor installers.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# verify_service_active SERVICE_NAME — checks systemd service is running.
verify_service_active() {
    _svc="$1"
    if systemctl is-active --quiet "$_svc" 2>/dev/null; then
        color_print green "[OK] Service ${_svc} is active"
        return 0
    else
        color_print red "[FAIL] Service ${_svc} is not active"
        echo ""
        echo "  Recent journal entries:"
        journalctl -u "$_svc" --no-pager -n 10 2>/dev/null || true
        echo ""
        return 1
    fi
}

# verify_port_listening PORT LABEL — checks if a TCP port is listening.
# Falls back through ss → netstat → /dev/tcp.
verify_port_listening() {
    _port="$1"
    _label="$2"

    if command -v ss >/dev/null 2>&1; then
        if ss -tlnp 2>/dev/null | grep -q ":${_port} "; then
            color_print green "[OK] ${_label} is listening on port ${_port}"
            return 0
        fi
    elif command -v netstat >/dev/null 2>&1; then
        if netstat -tlnp 2>/dev/null | grep -q ":${_port} "; then
            color_print green "[OK] ${_label} is listening on port ${_port}"
            return 0
        fi
    else
        # Last resort: try to connect.
        if (echo > /dev/tcp/127.0.0.1/"$_port") 2>/dev/null; then
            color_print green "[OK] ${_label} is listening on port ${_port}"
            return 0
        fi
    fi

    color_print red "[FAIL] ${_label} is not listening on port ${_port}"
    return 1
}

# verify_http_reachable URL LABEL — checks if an HTTP endpoint is reachable.
# Non-fatal if neither curl nor wget is available.
verify_http_reachable() {
    _url="$1"
    _label="$2"

    if command -v curl >/dev/null 2>&1; then
        if curl -skf -o /dev/null --connect-timeout 5 "$_url" 2>/dev/null; then
            color_print green "[OK] ${_label} is reachable at ${_url}"
            return 0
        else
            color_print red "[FAIL] ${_label} is not reachable at ${_url}"
            return 1
        fi
    elif command -v wget >/dev/null 2>&1; then
        if wget -q --spider --timeout=5 --no-check-certificate "$_url" 2>/dev/null; then
            color_print green "[OK] ${_label} is reachable at ${_url}"
            return 0
        else
            color_print red "[FAIL] ${_label} is not reachable at ${_url}"
            return 1
        fi
    fi

    color_print yellow "[SKIP] Cannot verify ${_label} (curl/wget not available)"
    return 0
}

# wait_and_verify_server GRPC_PORT DASHBOARD_PORT — full server verification.
# Waits for service startup, then checks service, ports, and dashboard.
wait_and_verify_server() {
    _grpc_port="$1"
    _dash_port="$2"
    _failures=0

    echo ""
    color_print bold "Verifying installation..."
    echo ""

    # Wait for service to start (up to 10 seconds).
    _waited=0
    while [ "$_waited" -lt 10 ]; do
        if systemctl is-active --quiet hitstor-server 2>/dev/null; then
            break
        fi
        sleep 1
        _waited=$((_waited + 1))
    done

    verify_service_active "hitstor-server" || _failures=$((_failures + 1))

    # Brief pause for ports to bind after service reports active.
    sleep 2

    verify_port_listening "$_grpc_port" "gRPC server" || _failures=$((_failures + 1))
    verify_http_reachable "https://127.0.0.1:${_dash_port}" "Dashboard" || _failures=$((_failures + 1))

    return "$_failures"
}

# wait_and_verify_agent — agent verification (service active only).
wait_and_verify_agent() {
    _failures=0

    echo ""
    color_print bold "Verifying installation..."
    echo ""

    # Wait for service to start (up to 10 seconds).
    _waited=0
    while [ "$_waited" -lt 10 ]; do
        if systemctl is-active --quiet hitstor-agent 2>/dev/null; then
            break
        fi
        sleep 1
        _waited=$((_waited + 1))
    done

    verify_service_active "hitstor-agent" || _failures=$((_failures + 1))

    return "$_failures"
}
