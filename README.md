# calypr-cli

[![CI](https://github.com/calypr/calypr-cli/actions/workflows/ci.yaml/badge.svg?branch=master)](https://github.com/calypr/calypr-cli/actions/workflows/ci.yaml)
[![Coverage](https://codecov.io/gh/calypr/calypr-cli/branch/develop/graph/badge.svg)](https://app.codecov.io/gh/calypr/calypr-cli/tree/develop)
[![Go Report Card](https://goreportcard.com/badge/github.com/calypr/calypr-cli)](https://goreportcard.com/report/github.com/calypr/calypr-cli)
[![Release](https://img.shields.io/github/v/release/calypr/calypr-cli?sort=semver)](https://github.com/calypr/calypr-cli/releases)

`calypr-cli` is the Calypr command-line client for data transfer, permissions,
collaboration, and portal operations.

`calypr-cli` is built on Cobra, a library providing a simple interface to create
powerful modern CLI interfaces similar to git and go tools. Read more about
Cobra [here](https://github.com/spf13/cobra).

## Calypr Extensions

This fork includes additional operator-focused command groups for permissions
management and portal configuration.

### Support Matrix

| Command group | Purpose | Supported surface | Not supported |
| --- | --- | --- | --- |
| `calypr-cli permissions` | Ownership, direct access, org membership, and auth mapping power tools | Public Gen3 `/authz` routes exposed through revproxy | Raw Arborist catalog/admin CRUD such as users, roles, resources, and arbitrary policy mutation |
| `calypr-cli portal` | Portal health checks and configuration management | Public `/gecko` routes exposed through revproxy | Direct backend-only Gecko paths or undocumented admin-only config flows |

### Guides

- [docs/permissions-cli.md](docs/permissions-cli.md): current permissions support,
  command examples, and deliberate non-support.
- [docs/portal-cli.md](docs/portal-cli.md): current portal support, command
  shapes, JSON payload expectations, and config-route behavior.

## Installation

First, [install Go](https://golang.org/doc/install) if you have not already.

The canonical install path for the renamed CLI is:

```bash
go install ./cmd/calypr-cli
```

From the repo root you can also build an explicit binary:

```bash
go build -o calypr-cli ./cmd/calypr-cli
```

## Enabling New Gen3 Object Management API

Some Gen3 data commons support uploading files through the new Gen3 Object Management API.

> NOTE: The service powering this API is sometimes referred to as our object "Shepherd"

To enable `calypr-cli` to upload using the Gen3 Object Management API, pass
`use-shepherd=true` to `calypr-cli configure`, for example:

```
$ calypr-cli configure --profile=myprofile --cred=/path/to/cred --apiendpoint=https://example.com --use-shepherd=true
```

If this flag is set, `calypr-cli` will attempt to use the Gen3 Object Management
API to upload files, falling back to Fence/Indexd in case of failure.

> You may also need to configure the version of the Gen3 Object Management API that the client will interact with. This is set to a default of Gen3 Object Management API `v2.0.0`, but can
> be raised or lowered by passing the `min-shepherd-version` flag to
> `calypr-cli configure`, e.g.:
>
> ```
> $ calypr-cli configure --profile=myprofile --cred=/path/to/cred --apiendpoint=https://example.com --use-shepherd=true --min-shepherd-version=1.3.0
> ```

### Uploading Additional File Object Metadata to Gen3 Object Management API

The Gen3 Object Management API supports uploading additional _public access_ file object metadata when uploading data files.

> WARNING: Additional File Object Metadata is exposed publically and thus should not be controlled/sensitive data

You can upload file metadata using the `calypr-cli upload` command with the
`--metadata` flag. For example:

```
calypr-cli upload --profile=my-profile --upload-path=/path/to/myfile.bam --metadata
```

This will tell `calypr-cli` to look for a metadata file `myfile_metadata.json` in
the same folder as `myfile.bam`.
A metadata file should be located in the same folder as the file to be uploaded, and should be named `[filename]_metadata.json`.

The metadata file should be a JSON file in the format:

```
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

The `aliases` and `metadata` properties are optional. Some Gen3 data commons require the `authz` property to be specified in order to upload a data file.

If you do not know what `authz` to use, you can look at your `Profile` tab or `/identity` page of the Gen3 data commons you are uploading to. You will see a list of _authz resources_ in the format `/example/authz/resource`: these are the authz resources you have access to.
