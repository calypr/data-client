# calypr-cli

[![CI](https://github.com/calypr/calypr-cli/actions/workflows/ci.yaml/badge.svg)](https://github.com/calypr/calypr-cli/actions/workflows/ci.yaml)
[![Coverage](https://github.com/calypr/calypr-cli/actions/workflows/coverage.yaml/badge.svg)](https://github.com/calypr/calypr-cli/actions/workflows/coverage.yaml)
[![Release](https://img.shields.io/github/v/release/calypr/calypr-cli?sort=semver)](https://github.com/calypr/calypr-cli/releases)

`calypr-cli` is Calypr's command-line client for working with Gen3-style data
commons from a terminal. It combines the standard data transfer flows with
operator-oriented surfaces for permissions, collaborators, and portal
configuration.

## What It Does

The CLI currently groups into four main areas:

| Command group | Purpose |
| --- | --- |
| `configure`, `auth` | Store credentials, create named profiles, and check connectivity |
| `upload*`, `download*`, `retry-upload` | Transfer files to and from supported backends |
| `collaborators` | Manage collaboration requests and approvals |
| `permissions`, `portal` | Operator workflows for Arborist-backed permissions and Gecko-backed portal config |

Most commands require a configured `--profile`, and the default backend is
`gen3`.

## Operator Surfaces

Two command groups are specific to the Calypr fork rather than the legacy data
transfer client:

| Command group | Supported surface | Deliberate non-goals |
| --- | --- | --- |
| `permissions` | Public Gen3 `/authz` routes exposed through revproxy | Raw Arborist catalog/admin CRUD and arbitrary policy mutation |
| `portal` | Public `/gecko` routes exposed through revproxy | Backend-only Gecko paths and undocumented admin-only flows |

Detailed operator docs live here:

- [permissions CLI guide](docs/permissions-cli.md)
- [portal CLI guide](docs/portal-cli.md)
- [developer docs](docs/DEVELOPER_DOCS.md)

## Install And Build

The repo keeps a root `main` package, so the primary local build path is the
repo root:

```bash
go build .
```

To install the current checkout:

```bash
go install .
```

There is also a compatibility entrypoint under `./cmd/calypr-cli` if you need a
subdirectory main package explicitly:

```bash
go build ./cmd/calypr-cli
```

## Common Setup

Configure a profile with credentials and an API endpoint:

```bash
calypr-cli configure \
  --profile=myprofile \
  --cred=/path/to/credentials.json \
  --apiendpoint=https://example.com
```

Check the resulting profile or auth state:

```bash
calypr-cli auth --profile=myprofile
```

## Upload Behavior

`calypr-cli` supports the newer Gen3 object management upload flow via the
`--use-shepherd=true` configure flag. When enabled, uploads attempt that API
first and fall back to the older Fence/Indexd path if needed.

Example:

```bash
calypr-cli configure \
  --profile=myprofile \
  --cred=/path/to/credentials.json \
  --apiendpoint=https://example.com \
  --use-shepherd=true
```

Uploads can also attach public file object metadata with `--metadata`. The CLI
looks for a sibling file named `[filename]_metadata.json`.

Example:

```bash
calypr-cli upload \
  --profile=myprofile \
  --upload-path=/path/to/myfile.bam \
  --metadata
```

Metadata file shape:

```json
{
  "authz": ["/example/authz/resource"],
  "aliases": ["example_alias"],
  "metadata": {
    "any": {
      "arbitrary": ["json", "metadata"]
    }
  }
}
```

The `authz` list may be required by the target commons. The metadata in this
file is public-facing and should not contain sensitive content.
