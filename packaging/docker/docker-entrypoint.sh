#!/bin/sh
set -e

# If the first argument looks like a flag, prepend "hitstor".
if [ "${1#-}" != "$1" ]; then
    set -- hitstor "$@"
fi

write_config() {
    if [ -z "$HITSTOR_DASHBOARD_PASSWORD" ] || [ -f /etc/hitstor/config.yaml ]; then
        return 0
    fi
    mkdir -p /etc/hitstor
    cat > /etc/hitstor/config.yaml <<EOF
serve_dashboard_password: "${HITSTOR_DASHBOARD_PASSWORD}"
EOF
}

auto_init_repo() {
    if [ "${HITSTOR_AUTO_INIT:-true}" != "true" ]; then
        return 0
    fi
    if [ -z "$HITSTOR_PASSWORD" ]; then
        echo "HITSTOR_AUTO_INIT is enabled but HITSTOR_PASSWORD is not set; skipping repository init"
        return 0
    fi

    repo="${HITSTOR_REPO:-}"
    if [ -z "$repo" ]; then
        return 0
    fi

    init_cmd="hitstor init"
    if [ -f /etc/hitstor/config.yaml ]; then
        init_cmd="$init_cmd --config /etc/hitstor/config.yaml"
    fi
    init_cmd="$init_cmd --repo $repo"

    case "$repo" in
        s3:*|plugin:*)
            echo "Auto-init for remote repository ($repo)..."
            $init_cmd || true
            ;;
        *)
            if [ ! -f "$repo/config" ]; then
                echo "Initializing local repository at $repo..."
                mkdir -p "$repo"
                $init_cmd
            fi
            ;;
    esac
}

if [ "$1" = "serve" ]; then
    shift
    write_config
    auto_init_repo

    if [ -f /etc/hitstor/config.yaml ]; then
        if [ -n "$HITSTOR_REPO" ]; then
            exec hitstor serve --config /etc/hitstor/config.yaml --repo "$HITSTOR_REPO" "$@"
        fi
        exec hitstor serve --config /etc/hitstor/config.yaml "$@"
    fi

    if [ -n "$HITSTOR_REPO" ]; then
        exec hitstor serve --repo "$HITSTOR_REPO" "$@"
    fi
    exec hitstor serve "$@"
fi

exec hitstor "$@"