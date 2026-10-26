# hitstor init

Initialize a new backup repository.

## Synopsis

```
hitstor init [flags]
```

## Description

Creates a new encrypted backup repository at the location specified by `--repo`. This sets up the directory structure (`data/`, `index/`, `snapshots/`, `keys/`, `locks/`, `config`) and generates a master key encrypted with the provided password using scrypt key derivation.

The repository must not already exist. The repository ID is printed on success.

## Flags

This command uses only the [global flags](index.md#global-flags).

## Examples

Initialize a local repository:

```bash
hitstor init --repo /backups/hitstor --password "my-secret-password"
```

Initialize an S3 repository:

```bash
hitstor init \
  --repo s3:http://localhost:9000/hitstor-backups \
  --password "my-secret-password" \
  --s3-access-key minioadmin \
  --s3-secret-key minioadmin
```

Using environment variables:

```bash
export HITSTOR_REPO=/backups/hitstor
export HITSTOR_PASSWORD="my-secret-password"
hitstor init
```
