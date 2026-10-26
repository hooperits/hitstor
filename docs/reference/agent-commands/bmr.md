# hitstor-agent bmr

Build and inspect bare-metal recovery media.

## Synopsis

```
hitstor-agent bmr check [flags]
hitstor-agent bmr generate-config [flags]
hitstor-agent bmr build-media [flags]
```

## Description

Identical command tree to [`hitstor bmr`](../commands/bmr.md) — both binaries share the same implementation, since recovery media can only be built on the host it protects, and `hitstor-agent` is what actually ships on a protected host.

**One difference**: on `hitstor-agent`, the `--server` flag's default value is pre-filled from the agent's own configured server address (the one it normally reports backups to), so `generate-config`/`build-media` need less to be typed on a machine that is already a registered agent. On `hitstor`, `--server` has no default.

See [hitstor bmr](../commands/bmr.md) for the full flag reference, subcommand descriptions, and examples — they apply here unchanged.

## Notes

- Recovery itself is driven by [hitstor-agent recover](recover.md), invoked automatically from the rescue media.
- See [Bare-metal restore](../../user-guide/bare-metal-restore.md) for the full workflow.
