# hitstor config

Manage configuration.

## Synopsis

```
hitstor config show [--json]
hitstor config set <key> <value>
hitstor config unset <key>
hitstor config add <key> <value>
hitstor config remove <key> <value>
hitstor config init
hitstor config edit
hitstor config path
hitstor config repositories list
```

## Description

Manage the HitStor configuration file at `~/.config/hitstor/config.yaml` (or `/etc/hitstor/config.yaml` on servers with --config).

For modern multi-repo management (repositories list, S3 creds, etc.), prefer the `hitstor repo` subcommand. The legacy flat keys here are for backward compat.

Configuration values can be set, unset, or viewed. List-type keys support `add` and `remove` operations.

## Subcommands

### `config show`

Display all configuration keys with their effective values and sources (default, config, env, flag). Sensitive values are masked. Use `--json` for machine-readable output.

### `config set <key> <value>`

Set a configuration key to a value. The value is validated before saving.

### `config unset <key>`

Remove a configuration key, reverting to its default value.

### `config add <key> <value>`

Append a value to a list-type key (e.g., `exclude`, `agent_paths`).

### `config remove <key> <value>`

Remove a specific value from a list-type key.

### `config init`

Interactive setup wizard. Walks through essential configuration options with prompts and examples.

### `config edit`

Open the configuration file in your default editor (`$EDITOR`).

### `config path`

Print the path to the configuration file. Prefers system `/etc/hitstor/config.yaml` on servers when present and readable.

### `config repositories list`

List modern repositories from config (equivalent to `hitstor repo list`). For management, use the `repo` subcommands.

## Examples

Show all config values:

```bash
hitstor config show
```

Set the repository location:

```bash
hitstor config set repo /var/lib/hitstor/repo
```

Add an exclude pattern:

```bash
hitstor config add exclude "*.tmp"
```

Run the interactive setup:

```bash
hitstor config init
```

## Notes

- Configuration file uses YAML format.
- Precedence: CLI flags > environment variables > config file > defaults.
- See [Config Reference](../config-reference.md) for the full list of keys.
