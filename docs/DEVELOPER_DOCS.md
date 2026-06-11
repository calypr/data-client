# Developer Docs

`calypr-cli` started from the Gen3 data-client lineage, but this repo now owns
its own CLI surface and operator workflows. Treat it as a command-oriented Go
application with a few backend-specific client packages, not as a thin mirror of
the old fork.

## Entry Points

- `main.go`: root-compatible binary entrypoint used by `go build .`
- `cmd/calypr-cli/main.go`: compatibility wrapper for subdirectory builds
- `cmd/`: Cobra command definitions and CLI orchestration

The root command is `calypr-cli`, and most subcommands require a named
`--profile`.

## Package Map

| Path | Responsibility |
| --- | --- |
| `cmd/` | User-facing commands for configure/auth, transfer flows, collaborators, permissions, and portal management |
| `conf/` | Profile loading, persistence, and config validation |
| `credentials/` | Credential refresh helpers |
| `g3client/` | Shared Gen3 client adapter layer, including Syfon-backed surfaces |
| `fence/` | Fence-specific request and response helpers |
| `gecko/` | Gecko-specific client code for portal operations |
| `arborist/` | Arborist-specific client code for permissions operations |

## High-Level Command Ownership

- Transfer commands live in `cmd/` and rely on shared Gen3/Syfon client code.
- `permissions` is the Arborist-backed operator surface.
- `portal` is the Gecko-backed operator surface.
- `collaborators` handles collaboration-request workflows exposed by the target
  commons.

## Build And Test

Local entrypoints:

```bash
go build .
go build ./cmd/calypr-cli
go test ./...
```

Top-level automation lives under `.github/workflows/`. The intended baseline is:

- `ci.yaml` for general lint and unit test coverage
- `coverage.yaml` for explicit coverage reporting
- `release.yaml` for tagged releases
- reusable image and integration workflows where the repo still depends on the
  shared `uc-cdis/.github` automation

## Notes

- The default backend flag is `gen3`, but the CLI also carries backend-specific
  behavior behind command flags and adapters.
- This repo still contains some compatibility surfaces from older Gen3 client
  behavior; when cleaning up, verify call sites before removing them.
