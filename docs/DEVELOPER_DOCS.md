# Dev Docs

This repo is a heavily updated / refactored version of https://github.com/uc-cdis/cdis-data-client

The new architecture splits out many of the mega packages into smaller, more digestable pieces. This whole CLI is essentially a Go client library for Gen3's Fence microservice.

These new packages are:

├── api
│   ├── gen3.go
│   └── types.go
├── client
│   └── client.go
├── common
│   ├── common.go
│   ├── constants.go
│   ├── isHidden_notwindows.go
│   ├── isHidden_windows.go
│   ├── logHelper.go
│   └── types.go
├── conf
│   ├── config.go
│   └── validate.go
├── download
│   ├── batch.go
│   ├── downloader.go
│   ├── file_info.go
│   ├── types.go
│   ├── url_resolution.go
│   └── utils.go
├── logs
│   ├── factory.go
│   ├── logger.go
│   ├── scoreboard.go
│   └── tee_logger.go
├── mocks
│   ├── mock_configure.go
│   ├── mock_functions.go
│   ├── mock_gen3interface.go
│   └── mock_request.go
├── request
│   ├── auth.go
│   ├── builder.go
│   └── request.go
└── upload
    ├── batch.go
    ├── multipart.go
    ├── request.go
    ├── retry.go
    ├── singleFile.go
    ├── types.go
    ├── upload.go
    └── utils.go


# api

This is the main Client API for talking to fence. Some of the functions that are currently defined in upload/ and download should probablyl be broken out into this library also.

# client

This is a thin wrapper client that wraps the API interface to make the API calls easier to use from other packages.

# common

This contains common constants / utility functions that are used in the repo

# conf

This is the config package for loading / storing the gen3 credential. Note ~/.gen3/.ini file is where credentials / configurations are stored,
but the raw credential is also stored in ~/.gen3/ under whatever you called it.

# download

This is the business logic for all download and download related operations in the depo

# logs

This is where the logger is defined

# mocks

This contains mocks for testing the data-client

# request

This is the lowest level interface for doing requests. It implements some basic retry, and wraps the http round trip with a token if one is provided

# upload

This contains the business logic for all upload and upload related operations.
