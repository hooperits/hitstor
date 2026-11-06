# hitstor version

Display version information and check for updates.

## Synopsis

```
hitstor version [--check] [--json]
```

## Description

Prints the HitStor version, commit hash, build date, and platform.

With `--check`, contacts the GitHub Releases API to determine if a newer version is available.

## Flags

| Flag | Description |
|------|-------------|
| `--check` | Check for available updates |
| `--json` | Output in JSON format |

## Examples

### Print version

```bash
hitstor version
```

Output:

```
hitstor v0.2.0 (commit a1b2c3d, built 2026-02-27, linux/amd64)
```

### Check for updates

```bash
hitstor version --check
```

Output when an update is available:

```
Update available: v0.3.0 (current: v0.2.0)
Release: https://github.com/hooperits/hitstor/releases/tag/v0.3.0
```

Output when up to date:

```
You are running the latest version (v0.2.0).
```

Output when the check fails (network error):

```
Could not check for updates.
```

## Automatic Version Check

When `update_check: true` is set in the server configuration (it is off unless set; **Settings → General** in the dashboard also turns it on), the server checks for updates every 24 hours. If a newer version is available, a notification banner appears at the top of the web dashboard.

See [Configuration Reference](../config-reference.md) for the `update_check` and `update_url` settings.
