# Gecko CLI

`data-client gecko` is the operator-facing CLI for the Gecko routes exposed
through the public Gen3 `/gecko` surface.

This is a configuration and health client. It is not a generic Gecko admin
shell for every internal backend route.

## Basic Shape

All commands use a normal data-client profile:

```bash
data-client --profile calypr gecko <command>
```

Add `--json` when you want raw output for debugging or scripts:

```bash
data-client --profile calypr gecko --json config types
```

## Current Support

`data-client gecko` currently supports:

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
data-client --profile calypr gecko health
```

## Generic Config Inspection

List the config types Gecko reports:

```bash
data-client --profile calypr gecko config types
```

List config IDs for a type:

```bash
data-client --profile calypr gecko config list explorer
data-client --profile calypr gecko config list nav
data-client --profile calypr gecko config list apps_page
```

Fetch a config by type and ID:

```bash
data-client --profile calypr gecko config get explorer default
data-client --profile calypr gecko config get nav default
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
data-client --profile calypr gecko project get HTAN_INT/BForePC
```

Create or replace a project config from JSON:

```bash
data-client --profile calypr gecko project put HTAN_INT/BForePC --file project.json
```

Delete a project config:

```bash
data-client --profile calypr gecko project delete HTAN_INT/BForePC
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
data-client --profile calypr gecko appcard get HTAN_INT-BForePC
```

Create or replace an app card from JSON:

```bash
data-client --profile calypr gecko appcard put HTAN_INT-BForePC --file appcard.json
```

Delete an app card:

```bash
data-client --profile calypr gecko appcard delete HTAN_INT-BForePC
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

`data-client gecko` does not try to expose every internal Gecko backend route.
It is currently scoped to the public revproxy-backed Gecko surface that this
client already knows how to model:

- health
- config type and config inspection
- project config CRUD
- app-card CRUD
