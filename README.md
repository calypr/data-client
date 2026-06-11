# data-client

[![CI](https://github.com/calypr/data-client/actions/workflows/ci.yaml/badge.svg?branch=master)](https://github.com/calypr/data-client/actions/workflows/ci.yaml)
[![Coverage](https://codecov.io/gh/calypr/data-client/branch/develop/graph/badge.svg)](https://app.codecov.io/gh/calypr/data-client/tree/develop)
[![Go Report Card](https://goreportcard.com/badge/github.com/calypr/data-client)](https://goreportcard.com/report/github.com/calypr/data-client)
[![Release](https://img.shields.io/github/v/release/calypr/data-client?sort=semver)](https://github.com/calypr/data-client/releases)

`data-client` is a command-line tool for downloading, uploading, and submitting data files to and from a Gen3 data commons.

Read more about what it does and how to use it in the `data-client` [user guide](https://gen3.org/resources/user/data-client/).

`data-client` is built on Cobra, a library providing a simple interface to create powerful modern CLI interfaces similar to git & go tools. Read more about Cobra [here](https://github.com/spf13/cobra).

## Calypr Extensions

This fork includes additional operator-focused command groups for Arborist and
Gecko.

### Support Matrix

| Command group | Purpose | Supported surface | Not supported |
| --- | --- | --- | --- |
| `data-client arborist` | Ownership, direct access, org membership, and auth mapping power tools | Public Gen3 `/authz` routes exposed through revproxy | Raw Arborist catalog/admin CRUD such as users, roles, resources, and arbitrary policy mutation |
| `data-client gecko` | Gecko health checks and configuration management | Public `/gecko` routes exposed through revproxy | Direct backend-only Gecko paths or undocumented admin-only config flows |

### Guides

- [docs/arborist-cli.md](docs/arborist-cli.md): current Arborist support,
  command examples, and deliberate non-support.
- [docs/gecko-cli.md](docs/gecko-cli.md): current Gecko support, command
  shapes, JSON payload expectations, and config-route behavior.

## Installation

(The following instruction is for compiling and installing the `data-client` from source code. There are also binary executables can be found at [here](https://github.com/uc-cdis/cdis-data-client/releases))

First, [install Go and the Go tools](https://golang.org/doc/install) if you have not already done so. [Set up your workspace and your GOPATH.](https://golang.org/doc/code.html)

Then:

```
go get -d github.com/calypr/data-client
go install
```

_TODO: Remove after GitHub repo is renamed_
**_For now, the above actually won't work because the GitHub repo needs to be renamed. Do this instead:_**

```
mkdir -p $GOPATH/src/github.com/uc-cdis
cd $GOPATH/src/github.com/uc-cdis
git clone git@github.com:uc-cdis/cdis-data-client.git
mv cdis-data-client data-client
cd data-client
go get -d ./...
go install .
```

Now you should have `data-client` successfully installed. For a comprehensive instruction on how to configure and use `data-client` for uploading / downloading object files, please refer to the `data-client` [user guide](https://gen3.org/resources/user/data-client/).

## Enabling New Gen3 Object Management API

Some Gen3 data commons support uploading files through the new Gen3 Object Management API.

> NOTE: The service powering this API is sometimes referred to as our object "Shepherd"

To enable data-client to upload using the Gen3 Object Management API, pass the `use-shepherd=true` to `data-client configure`, e.g.:

```
$ data-client configure --profile=myprofile --cred=/path/to/cred --apiendpoint=https://example.com --use-shepherd=true
```

If this flag is set, the data-client will attempt to use the Gen3 Object Management API to upload files, falling back to Fence/Indexd in case of failure.

> You may also need to configure the version of the Gen3 Object Management API that the client will interact with. This is set to a default of Gen3 Object Management API `v2.0.0`, but can
> be raised or lowered by passing the `min-shepherd-version` flag to `data-client configure`, e.g.:
>
> ```
> $ data-client configure --profile=myprofile --cred=/path/to/cred --apiendpoint=https://example.com --use-shepherd=true --min-shepherd-version=1.3.0
> ```

### Uploading Additional File Object Metadata to Gen3 Object Management API

The Gen3 Object Management API supports uploading additional _public access_ file object metadata when uploading data files.

> WARNING: Additional File Object Metadata is exposed publically and thus should not be controlled/sensitive data

You can upload file metadata using the `data-client upload` command with the `--metadata` flag. E.g.:

```
data-client upload --profile=my-profile --upload-path=/path/to/myfile.bam --metadata
```

This will tell `data-client` to look for a metadata file `myfile_metadata.json` in the same folder as `myfile.bam`.
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
