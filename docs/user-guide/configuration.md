# Configuration

HitStor can be configured through CLI flags, environment variables, and a YAML configuration file. This page explains how these sources interact, how to manage the config file, and how to use the interactive setup wizard. For the full list of keys, see the [Configuration Reference](../reference/config-reference.md).

## Config File Location

When `--config` is not given, HitStor uses the first of these that exists and is readable:

1. `/etc/hitstor/config.yaml` -- the system-wide file used by the server service, the packages, and the installers.
2. `~/.config/hitstor/config.yaml` -- the per-user file (`%AppData%\hitstor\config.yaml` on Windows).

If neither exists, the per-user path is used, and `hitstor config init` or `hitstor config set` creates it there. Pass `--config <path>` to use any other file; `hitstor config path` prints the file in use.

## Precedence

When the same setting is specified in multiple places, HitStor applies the following precedence (highest to lowest):

1. **CLI flags** -- always win.
2. **Config file** -- provides persistent defaults.
3. **Environment variables** -- used when neither a flag nor the config file sets the value.
4. **Built-in defaults** -- used when nothing else is specified.

For example, if the config file sets `repo: /backups/local` but you pass `--repo /backups/remote` on the command line, the CLI flag takes effect. A `HITSTOR_REPO` environment variable is only used when neither sets the repository.

The repository password has no config file key of its own. It is taken from `--password`, then `HITSTOR_PASSWORD`, then `--password-file`, then the `password_file` of the repository selected with `--repo-name`.

## Environment Variables

The following environment variables are recognized by the `hitstor` CLI:

| Variable | Purpose | Example |
|----------|---------|---------|
| `HITSTOR_REPO` | Default repository location | `export HITSTOR_REPO=/path/to/repo` |
| `HITSTOR_PASSWORD` | Repository password | `export HITSTOR_PASSWORD=secret` |

See [Environment Variables](../reference/environment-variables.md) for the variables read by the server and the agent.

!!! warning
    Avoid setting `HITSTOR_PASSWORD` in shell profiles or scripts stored in version control. Prefer `--password-file` (or a repository's `password_file`) pointing to a file with strict permissions (e.g., `chmod 600`).

## Config Commands

HitStor provides a set of subcommands for managing the configuration file without editing YAML by hand.

### `config show`

Display the current configuration:

```console
hitstor config show
Config file: /home/user/.config/hitstor/config.yaml

Configured repositories (1):
  [0] default
      Type:           local
      Path:           /backups/local
      Password file:  /etc/hitstor/password.txt [exists, readable]

Server settings (from this file):
  (none customized in this file)

Legacy single-repo keys (for backward compatibility):
  exclude                  *.tmp, node_modules  config
  parallel                 4                    config
```

Sensitive values such as S3 keys are masked in the output. `config show --json` lists every key with its value and source.

### `config set`

Set a single configuration key:

```console
hitstor config set repo /backups/local
hitstor config set parallel 4
```

Keys are flat names (`repo`, `password_file`, `parallel`, `exclude`, `s3_region`, `serve_port`, and so on); the command lists the valid keys when given an unknown one.

### `config unset`

Remove a configuration key:

```console
hitstor config unset parallel
```

### `config add`

Append a value to a list-type key:

```console
hitstor config add exclude "*.log"
hitstor config add exclude ".cache"
```

### `config remove`

Remove a value from a list-type key:

```console
hitstor config remove exclude "*.log"
```

### `config init`

Launch the interactive setup wizard (see below).

### `config edit`

Open the config file in your editor (`$EDITOR`, then `$VISUAL`, else `vi`):

```console
hitstor config edit
```

### `config path`

Print the path to the config file:

```console
hitstor config path
/home/user/.config/hitstor/config.yaml
```

## Interactive Setup Wizard

The `config init` command walks you through the essential settings interactively. It needs an interactive terminal:

```console
hitstor config init
Welcome to HitStor configuration setup!

Backend type (local/s3) [local]:
Repository path: /backups/local
Password file (optional): /etc/hitstor/password.txt
Parallel workers (0 = auto) [0]: 4
Exclude patterns (comma-separated, optional): *.tmp, node_modules

Configuration saved to /home/user/.config/hitstor/config.yaml
```

For `s3` it asks for the repository (`s3:endpoint/bucket`), the S3 access key, secret key, and region instead of a path. If a config file already exists, the wizard asks before overwriting it and offers the current values as defaults.

## Example Configurations

### Local Repository

A minimal config for backing up to a local disk:

```yaml
repo: /mnt/external/hitstor-repo
password_file: /etc/hitstor/password.txt
parallel: 4
exclude:
  - "*.tmp"
  - ".cache"
  - "node_modules"
```

When both `repo` and `password_file` are set, HitStor treats them as a repository named `default`. Select it with `--repo-name default` so the password is read from the file, or pass `--password-file` yourself:

```console
hitstor --repo-name default backup /data
```

### S3 Repository

Backing up to an S3-compatible bucket:

```yaml
repo: s3:https://s3.amazonaws.com/my-bucket
s3_access_key: AKIAIOSFODNN7EXAMPLE
s3_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
s3_region: us-east-1
parallel: 8
exclude:
  - "*.tmp"
  - ".cache"
```

The repository uses the whole bucket. S3 credentials come from the `s3_*` keys or the `--s3-access-key`, `--s3-secret-key`, and `--s3-region` flags; the AWS environment variables and shared credentials file are not read. Give the repository password with `--password-file` or `HITSTOR_PASSWORD`.

### Remote Agent Setup

The agent (`hitstor-agent`) reads `agent_*` keys. A dashboard-managed agent needs none of them: registration stores the server address and API key in `agent.env`. For an agent registered by hand:

```yaml
agent_server: backup.example.com:8420
agent_api_key: a1b2c3d4...   # 64-char hex, from `hitstor serve agent add <name>`
agent_ca_cert: /etc/hitstor/ca.pem
agent_hostname: webserver-01
agent_paths:
  - /etc
  - /var/www
agent_excludes:
  - "*.tmp"
```

On the server, `alert_notifier_plugins` and `alert_webhook_url` configure alert delivery; see [Plugins](plugins.md#notifier-plugins).

## Sensitive Values

HitStor takes care not to expose sensitive configuration values:

- `config show` masks secrets such as S3 keys.
- The config file itself is not encrypted, except for values the server stores encrypted with the repository key (for example the S3 keys of repositories created from the dashboard, and email and integration secrets). Protect it with appropriate file-system permissions (`chmod 600 ~/.config/hitstor/config.yaml`).
- Prefer `password_file` (or `--password-file`) over passing the password on the command line. The file should also have strict permissions.

!!! tip
    You can combine the config file with environment variables for a layered approach: store non-sensitive defaults in the config file and inject the password through `HITSTOR_PASSWORD` at runtime.
