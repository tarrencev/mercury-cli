# mercury-cli

Go CLI for the Mercury Bank API, generated from Mercury's published OpenAPI specs.

## Install / Build

```bash
go build ./...
go build -o mercury .
```

## Install With asdf

This repo includes an asdf plugin so you can install published release binaries:

```bash
asdf plugin add mercury-cli https://github.com/tarrencev/mercury-cli.git
asdf list all mercury-cli
asdf install mercury-cli <version>
asdf global mercury-cli <version>

mercury version
```

## Auth

By default, secured endpoints require a token:

```bash
export MERCURY_TOKEN="..."
```

Or pass `--token` explicitly.

Auth schemes:
- `--auth bearer` (default): `Authorization: Bearer <token>`
- `--auth basic`: `Authorization: Basic base64(<token>:)`

## Usage

Commands are generated from OpenAPI tags and `operationId`s:

```bash
mercury <group> <operation> [path-args...] [--query-flags...]
```

Examples:

```bash
# List accounts (cursor pagination supported)
mercury accounts get-accounts --limit 100

# Fetch all pages
mercury accounts get-accounts --all

# NDJSON output for scripting
mercury --ndjson accounts get-accounts --all

# Get one account by ID
mercury accounts get-account acc_123

# Create a recipient (JSON body)
mercury recipients create-recipient --data @recipient.json

# Upload a recipient attachment (multipart/form-data)
mercury recipients upload-recipient-attachment r_123 \
  --form note=hi \
  --form file=@./doc.pdf
```

## Environments

```bash
# Default: prod
mercury --env prod accounts get-accounts

# Sandbox
mercury --env sandbox accounts get-accounts
```

Advanced: override the server base URL (useful for testing against a proxy or `httptest`):

```bash
mercury --base-url http://localhost:8080/api/v1 accounts get-accounts
```

## Spec Maintenance

Specs are vendored in `specs/*.json` and embedded into the binary.

```bash
mercury spec list
mercury spec verify
mercury spec update

# convenience wrapper
./bin/spec-update
```

## Releases

Tag a release like `v0.1.0` to build and publish cross-platform binaries via GitHub Actions.
