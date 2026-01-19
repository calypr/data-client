# data-client

[![Build Status](https://travis-ci.org/uc-cdis/cdis-data-client.svg?branch=master)](https://travis-ci.org/uc-cdis/cdis-data-client)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/uc-cdis/cdis-data-client?sort=semver)](https://github.com/uc-cdis/cdis-data-client/releases)

`data-client` is a command-line tool for downloading, uploading, and submitting data files to and from a Gen3 data commons.

Read more about what it does and how to use it in the `data-client` [user guide](https://gen3.org/resources/user/data-client/).

`data-client` is built on Cobra, a library providing a simple interface to create powerful modern CLI interfaces similar to git & go tools. Read more about Cobra [here](https://github.com/spf13/cobra).

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

## Multipart Upload

The `data-client` supports multipart upload for large files, which splits files into smaller chunks (parts) for more reliable uploads with resume capability.

### Chunk Size (Message Size) in Multipart Uploads

When uploading files using multipart upload, the file is divided into chunks (also referred to as "parts" or "messages"). The chunk size is automatically determined based on the file size:

- **For files ≤ 512 MB**: 32 MB chunks
- **For files 512 MB - ~49 GB**: 5 MB chunks (S3 minimum)
  - This threshold (~49 GB = 10,000 parts × 5 MB) is where files hit the S3 limit of 10,000 parts when using the minimum chunk size
- **For files > ~49 GB**: Dynamically calculated to stay within S3's limit of 10,000 parts per upload
  - Minimum chunk size: 5 MB (S3 requirement)
  - Maximum number of parts: 10,000
  - Chunk sizes are rounded up to the nearest MB for efficiency

**Example chunk sizes:**
- 100 MB file → 32 MB chunks (4 parts)
- 1 GB file → 5 MB chunks (~205 parts)
- 10 GB file → 5 MB chunks (~2,048 parts)
- 50 GB file → 6 MB chunks (~8,534 parts)
- 100 GB file → 11 MB chunks (~9,310 parts)
- 1 TB file → 105 MB chunks (~9,987 parts)

The multipart upload process runs up to 10 concurrent part uploads for optimal performance, with automatic retry logic for failed parts.
