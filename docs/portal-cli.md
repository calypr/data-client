# Portal CLI

`calypr-cli portal` is the operator-facing CLI for the portal routes exposed
through the public Gen3 `/gecko` surface.

This is a configuration and health client. It is not a generic Gecko admin
shell for every internal backend route.

## Basic Shape

All commands use a normal calypr-cli profile:

```bash
calypr-cli --profile calypr portal <command>
```

Add `--json` when you want raw output for debugging or scripts:

```bash
calypr-cli --profile calypr portal --json config types
```

## Current Support

`calypr-cli portal` currently supports:

- service health checks
- listing Gecko config types
- listing config IDs for a config type
- fetching a config by type and ID
- project config get, put, and delete
- app-card get, put, and delete

The client currently recognizes these Gecko config-type tokens:

- `explorer`
- `nav`
- `file_summary`
- `apps_page`
- `project`
- `projects`

`project` config CRUD should use the dedicated `project` commands below. Those
commands target the public `/gecko/projects/...` route shape.

## Health Check

Check whether Gecko is reachable through revproxy:

```bash
calypr-cli --profile calypr portal health
```

## Generic Config Inspection

List the config types Gecko reports:

```bash
calypr-cli --profile calypr portal config types
```

List config IDs for a type:

```bash
calypr-cli --profile calypr portal config list explorer
calypr-cli --profile calypr portal config list nav
calypr-cli --profile calypr portal config list apps_page
```

Fetch a config by type and ID:

```bash
calypr-cli --profile calypr portal config get explorer default
calypr-cli --profile calypr portal config get nav default
```

Use `config` when you need typed config inspection. Use the dedicated commands
below when you are working with project configs or app cards.

## Project Configs

Project config commands use an ID in `ORG/PROJECT` form, not a flat project
name.

This is valid:

```bash
HTAN_INT/BForePC
```

This is invalid:

```bash
BForePC
```

Get a project config:

```bash
calypr-cli --profile calypr portal project get HTAN_INT/BForePC
```

Create or replace a project config from JSON:

```bash
calypr-cli --profile calypr portal project put HTAN_INT/BForePC --file project.json
```

Delete a project config:

```bash
calypr-cli --profile calypr portal project delete HTAN_INT/BForePC
```

Expected JSON shape:

```json
{
  "title": "BForePC",
  "contact_email": "owner@example.org",
  "src_repo": "https://github.com/calypr/gecko",
  "org_title": "HTAN INT",
  "description": "Example project config",
  "project_title": "BForePC",
  "icon_name": "folder-git-2"
}
```

`src_repo` is validated before the request is sent. The client expects a real
GitHub repository and normalizes the value before writing the config.

## App Cards

App-card commands use a project ID, usually in `ORG-PROJECT` form.

Get an app card:

```bash
calypr-cli --profile calypr portal appcard get HTAN_INT-BForePC
```

Create or replace an app card from JSON:

```bash
calypr-cli --profile calypr portal appcard put HTAN_INT-BForePC --file appcard.json
```

Delete an app card:

```bash
calypr-cli --profile calypr portal appcard delete HTAN_INT-BForePC
```

Expected JSON shape:

```json
{
  "title": "BForePC",
  "description": "Open the BForePC workspace",
  "icon": "folder-git-2",
  "href": "/workspace/BForePC",
  "perms": "read"
}
```

## Deliberate Non-Support

`calypr-cli portal` does not try to expose every internal Gecko backend route.
It is currently scoped to the public revproxy-backed Gecko surface that this
client already knows how to model:

- health
- config type and config inspection
- project config CRUD
- app-card CRUD

The legacy backend-oriented command name `calypr-cli gecko` still works as a
compatibility alias, but `portal` is the supported user-facing name.
