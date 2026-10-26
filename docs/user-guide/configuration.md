# Configuration

HitStor can be configured through CLI flags, environment variables, and a YAML configuration file. This page explains how these sources interact, how to manage the config file, and how to use the interactive setup wizard.

## Config File Location

The default configuration file is located at:

```
~/.config/hitstor/config.yaml
```

HitStor creates this directory automatically the first time you run `hitstor config init`. You can also create the file manually.

## Precedence

When the same setting is specified in multiple places, HitStor applies the following precedence (highest to lowest):

1. **CLI flags** -- always win.
2. **Environment variables** -- override the config file.
3. **Config file** -- provides persistent defaults.
4. **Built-in defaults** -- used when nothing else is specified.

For example, if the config file sets `repo: /backups/local` but you pass `--repo /backups/remote` on the command line, the CLI flag takes effect.

## Environment Variables

The following environment variables are recognized:

| Variable | Purpose | Example |
|----------|---------|---------|
| `HITSTOR_REPO` | Default repository location | `export HITSTOR_REPO=/path/to/repo` |
| `HITSTOR_PASSWORD` | Repository password | `export HITSTOR_PASSWORD=secret` |
| `HITSTOR_PASSWORD_FILE` | Path to a file containing the password | `export HITSTOR_PASSWORD_FILE=/etc/hitstor/password.txt` |

When both `HITSTOR_PASSWORD` and `HITSTOR_PASSWORD_FILE` are set, `HITSTOR_PASSWORD` takes precedence.

!!! warning
    Avoid setting `HITSTOR_PASSWORD` in shell profiles or scripts stored in version control. Use `HITSTOR_PASSWORD_FILE` with strict file permissions (e.g., `chmod 600`) for better security.

## Config Commands

HitStor provides a set of subcommands for managing the configuration file without editing YAML by hand.

### `config show`

Display the current configuration:

```console
hitstor config show
repo: /backups/local
password: ********
backup:
  exclude:
    - "*.tmp"
    - "node_modules"
  parallel: 4
```

Sensitive values such as passwords are masked in the output.

### `config set`

Set a single configuration key:

```console
hitstor config set repo /backups/local
hitstor config set backup.parallel 4
```

Dot notation is used for nested keys.

### `config unset`

Remove a configuration key:

```console
hitstor config unset backup.parallel
```

### `config add`

Append a value to a list-type key:

```console
hitstor config add backup.exclude "*.log"
hitstor config add backup.exclude ".cache"
```

### `config remove`

Remove a value from a list-type key:

```console
hitstor config remove backup.exclude "*.log"
```

### `config init`

Launch the interactive setup wizard (see below).

### `config edit`

Open the config file in your default editor (`$EDITOR` or `vi`):

```console
hitstor config edit
```

### `config path`

Print the absolute path to the config file:

```console
hitstor config path
/home/user/.config/hitstor/config.yaml
```

## Interactive Setup Wizard

The `config init` command walks you through the essential settings interactively:

```console
hitstor config init
Welcome to HitStor configuration.

Repository path []: /backups/local
Password []: ********
Default number of parallel workers (0 = auto) [0]: 4

Exclude patterns (one per line, empty line to finish):
> *.tmp
> node_modules
>

Configuration saved to /home/user/.config/hitstor/config.yaml
```

The wizard creates the config directory and file if they do not exist. If a config file already exists, the wizard pre-fills current values so you can update them selectively.

## Example Configurations

### Local Repository

A minimal config for backing up to a local disk:

```yaml
repo: /mnt/external/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
    - ".cache"
    - "node_modules"
  parallel: 4
```

### S3 Repository

Backing up to an S3-compatible bucket:

```yaml
repo: s3:s3.amazonaws.com/my-bucket/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
    - ".cache"
  parallel: 8
```

AWS credentials are configured through the standard environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) or the shared credentials file.

### Remote Agent Setup

When HitStor communicates with a remote agent for server-side processing:

```yaml
repo: agent:user@backupserver:/data/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
  parallel: 4
  host: webserver-01

alerting:
  alert_notifier_plugins:
    - slack-notifier
```

## Sensitive Values

HitStor takes care not to expose sensitive configuration values:

- `config show` masks passwords and tokens with `********`.
- The config file itself is not encrypted. Protect it with appropriate file-system permissions (`chmod 600 ~/.config/hitstor/config.yaml`).
- Prefer `password_file` over storing the password directly in the config file. The file referenced by `password_file` should also have strict permissions.

!!! tip
    You can combine the config file with environment variables for a layered approach: store non-sensitive defaults in the config file and inject passwords through `HITSTOR_PASSWORD_FILE` at runtime.
