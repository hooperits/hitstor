# common.sh — Shared installer library for HitStor v2 installers.
# Inlined at build time by build-installer.sh. POSIX sh compatible.

# ---- Colors ----

_use_color=0
if [ -t 1 ]; then
    _use_color=1
fi

color_print() {
    _color="$1"; shift
    if [ "$_use_color" -eq 0 ]; then
        printf "%s\n" "$*"
        return
    fi
    case "$_color" in
        green)  printf "\033[32m%s\033[0m\n" "$*" ;;
        red)    printf "\033[31m%s\033[0m\n" "$*" ;;
        yellow) printf "\033[33m%s\033[0m\n" "$*" ;;
        blue)   printf "\033[34m%s\033[0m\n" "$*" ;;
        bold)   printf "\033[1m%s\033[0m\n" "$*" ;;
        *)      printf "%s\n" "$*" ;;
    esac
}

# ---- Banner ----

banner() {
    _component="$1"  # "Server" or "Agent"
    echo ""
    color_print bold "============================================="
    color_print bold "  HitStor ${_component} v${HITSTOR_VERSION}"
    color_print bold "============================================="
    echo ""
}

# ---- Flag Parsing ----

FLAG_UNINSTALL=0
FLAG_NO_WIZARD=0
FLAG_YES=0
FLAG_VERSION=0
FLAG_HELP=0

parse_flags() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --uninstall)  FLAG_UNINSTALL=1 ;;
            --no-wizard)  FLAG_NO_WIZARD=1 ;;
            --yes)        FLAG_YES=1 ;;
            --version)    FLAG_VERSION=1 ;;
            --help|-h)    FLAG_HELP=1 ;;
            *)
                echo "Unknown option: $1" >&2
                echo "Usage: $0 [--uninstall] [--no-wizard] [--yes] [--version] [--help]" >&2
                exit 1
                ;;
        esac
        shift
    done
}

show_help() {
    _component="$1"
    cat <<EOF
HitStor ${_component} Installer v${HITSTOR_VERSION}

Usage: $0 [OPTIONS]

Options:
  --no-wizard   Non-interactive install (use environment variables)
  --uninstall   Remove HitStor ${_component} from this system
  --yes         Skip confirmation prompts (use with --uninstall)
  --version     Show installer version and exit
  --help, -h    Show this help message

EOF
}

# ---- Prerequisite Checks ----

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        color_print red "Error: this installer must be run as root (use sudo)"
        exit 1
    fi
}

check_linux() {
    if [ "$(uname -s)" != "Linux" ]; then
        color_print red "Error: this installer is for Linux only"
        exit 1
    fi
}

check_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        color_print red "Error: systemd is required but not found"
        color_print yellow "Hint: HitStor requires a systemd-based Linux distribution"
        exit 1
    fi
}

# grant_hitstor_journal_access adds hitstor to systemd-journal so View Logs can read journald.
grant_hitstor_journal_access() {
    if ! getent passwd hitstor >/dev/null 2>&1; then
        return 0
    fi
    if ! getent group systemd-journal >/dev/null 2>&1; then
        return 0
    fi
    if id -nG hitstor 2>/dev/null | tr ' ' '\n' | grep -qx systemd-journal; then
        return 0
    fi
    if usermod -aG systemd-journal hitstor 2>/dev/null; then
        color_print green "[+] Granted journal access for View Logs (systemd-journal group)"
    else
        color_print yellow "[!] Could not add hitstor to systemd-journal (View Logs may show journal warnings)"
    fi
}

# ---- Payload Extraction ----

extract_payload() {
    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"; stty echo 2>/dev/null' EXIT INT TERM
    ARCHIVE=$(awk '/^__PAYLOAD_BELOW__$/{print NR + 1; exit 0;}' "$0")
    tail -n +"$ARCHIVE" "$0" | tar xz -C "$TMPDIR"
}

# ---- Prompt Functions ----

# prompt_value VARNAME "Label" "default"
# Sets the variable named VARNAME to the user's input (or default).
prompt_value() {
    _var="$1"
    _label="$2"
    _default="$3"

    if [ -n "$_default" ]; then
        printf "  %s [%s]: " "$_label" "$_default"
    else
        printf "  %s: " "$_label"
    fi
    read -r _input
    if [ -z "$_input" ]; then
        _input="$_default"
    fi
    eval "$_var=\"\$_input\""
}

# prompt_password VARNAME "Label" MIN_LEN
# Reads a password with masked input. If MIN_LEN > 0, validates length
# and requires confirmation (enter twice).
prompt_password() {
    _var="$1"
    _label="$2"
    _min_len="$3"

    while true; do
        printf "  %s: " "$_label"
        stty -echo
        read -r _pass
        stty echo
        printf "\n"

        # Allow empty if min_len is 0 (optional password).
        if [ "$_min_len" -eq 0 ] && [ -z "$_pass" ]; then
            eval "$_var=''"
            return
        fi

        # Validate minimum length.
        if [ "$_min_len" -gt 0 ]; then
            _len=$(printf "%s" "$_pass" | wc -c)
            if [ "$_len" -lt "$_min_len" ]; then
                color_print red "    Password must be at least ${_min_len} characters"
                continue
            fi
        fi

        # Confirm by entering again.
        printf "  Confirm %s: " "$_label"
        stty -echo
        read -r _pass2
        stty echo
        printf "\n"

        if [ "$_pass" != "$_pass2" ]; then
            color_print red "    Passwords do not match, please try again"
            continue
        fi

        eval "$_var=\"\$_pass\""
        return
    done
}

# prompt_yesno "Question" "default" — returns 0 for yes, 1 for no.
# default is "y" or "n".
prompt_yesno() {
    _question="$1"
    _default="$2"

    if [ "$FLAG_YES" -eq 1 ]; then
        return 0
    fi

    if [ "$_default" = "y" ]; then
        _hint="[Y/n]"
    else
        _hint="[y/N]"
    fi

    printf "  %s %s: " "$_question" "$_hint"
    read -r _answer

    if [ -z "$_answer" ]; then
        _answer="$_default"
    fi

    case "$_answer" in
        [Yy]|[Yy][Ee][Ss]) return 0 ;;
        *) return 1 ;;
    esac
}

# ---- Disk Space ----

# show_disk_space PATH — shows available disk space at the given path.
# Walks up to the nearest existing parent directory if path doesn't exist.
show_disk_space() {
    _path="$1"
    _check="$_path"
    while [ ! -d "$_check" ] && [ "$_check" != "/" ]; do
        _check=$(dirname "$_check")
    done
    _avail=$(df -h "$_check" 2>/dev/null | awk 'NR==2{print $4}')
    if [ -n "$_avail" ]; then
        color_print blue "    Available disk space: ${_avail}"
    fi
}

# ---- Validation Functions ----

# validate_port VALUE — returns 0 if valid port (1-65535), 1 otherwise.
validate_port() {
    _port="$1"
    case "$_port" in
        ''|*[!0-9]*) return 1 ;;
    esac
    [ "$_port" -ge 1 ] && [ "$_port" -le 65535 ]
}

# validate_hex64 VALUE — returns 0 if exactly 64 hex characters.
validate_hex64() {
    _val="$1"
    _len=$(printf "%s" "$_val" | wc -c)
    if [ "$_len" -ne 64 ]; then
        return 1
    fi
    case "$_val" in
        *[!0-9a-fA-F]*) return 1 ;;
    esac
    return 0
}

# detect_outbound_ip — prints the server's preferred outbound IP address.
# Falls back to 127.0.0.1 if detection fails.
detect_outbound_ip() {
    # Try ip route (most Linux systems).
    _ip=$(ip route get 8.8.8.8 2>/dev/null | grep -oP 'src \K\S+' | head -1)
    if [ -n "$_ip" ]; then
        printf "%s" "$_ip"
        return
    fi
    # Fallback: hostname -I (first IP).
    _ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    if [ -n "$_ip" ]; then
        printf "%s" "$_ip"
        return
    fi
    printf "127.0.0.1"
}

# validate_host_port VALUE — returns 0 if "host:port" format.
validate_host_port() {
    _val="$1"
    case "$_val" in
        *:*)
            _host="${_val%:*}"
            _port="${_val##*:}"
            if [ -z "$_host" ]; then
                return 1
            fi
            validate_port "$_port"
            return $?
            ;;
        *) return 1 ;;
    esac
}
