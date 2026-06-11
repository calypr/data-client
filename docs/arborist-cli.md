# Arborist Power Tools CLI

`data-client arborist` is the operator-facing CLI for the Arborist routes that
are actually exposed through the public Gen3 `/authz` surface.

This is intentionally not a full Arborist admin client. Raw Arborist catalog
routes like user, role, and resource CRUD are not exposed through revproxy, so
`data-client` does not try to support them.

Use `data-client collaborators` when a user is asking for access they do not
already have. Use `data-client arborist` when an admin needs to inspect or
change the ownership and direct-access state that Arborist exposes publicly.

## Basic Shape

All commands use the normal data-client profile:

```bash
data-client --profile calypr arborist <command>
```

Add `--json` when you need raw output for debugging or scripts:

```bash
data-client --profile calypr arborist --json auth mapping
```

## Current Auth Mapping

Show the current profile user's Arborist mapping:

```bash
data-client --profile calypr arborist auth mapping
```

This command reads the public `GET /authz/mapping` surface for the current
token. It does not support arbitrary username lookups.

## Organization Membership

Organization membership is a convenience wrapper around Arborist direct-access
grants on `/programs/<org>/projects`.

```bash
data-client --profile calypr arborist org-membership add user@ohsu.edu Ellrott_Lab
data-client --profile calypr arborist org-membership rm user@ohsu.edu Ellrott_Lab
```

The default role is `org-member`. It carries only
`arborist/create-descendant`, which lets the member create projects under the
existing organization without granting ownership or access on existing projects.

You can specify another role when needed:

```bash
data-client --profile calypr arborist org-membership add user@ohsu.edu Ellrott_Lab --role org-member
```

Do not pass a resource path to `org-membership`. This is valid:

```bash
Ellrott_Lab
```

This is invalid:

```bash
/programs/Ellrott_Lab
```

## Ownership Power Tools

Create a new organization resource and make the caller its owner:

```bash
data-client --profile calypr arborist ownership create-descendant \
  --parent /programs \
  --name Ellrott_Lab \
  --template gen3-program
```

Create a new project resource under an organization and make the caller its
owner:

```bash
data-client --profile calypr arborist ownership create-descendant \
  --parent /programs/Ellrott_Lab/projects \
  --name git_drs_test \
  --template gen3-project
```

Add or remove owners:

```bash
data-client --profile calypr arborist ownership add-owner \
  --resource /programs/Ellrott_Lab \
  --user user@ohsu.edu

data-client --profile calypr arborist ownership rm-owner \
  --resource /programs/Ellrott_Lab \
  --user user@ohsu.edu
```

Read the normalized ownership and direct-access state for a resource:

```bash
data-client --profile calypr arborist ownership get-resource \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --include-admins \
  --include-children
```

## Direct Access Power Tools

Grant or revoke direct non-owner access on an existing resource:

```bash
data-client --profile calypr arborist access grant-user \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --user user@ohsu.edu \
  --role writer

data-client --profile calypr arborist access revoke-user \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --user user@ohsu.edu \
  --role writer
```

Use these commands for ordinary direct access. Use ownership add/remove for
owner changes.

## Deliberate Non-Support

`data-client arborist` does not support raw Arborist catalog/admin routes such
as:

- user CRUD
- role CRUD
- resource CRUD
- raw policy mutation
- arbitrary-user auth mapping lookup

Those routes are not part of the supported public Gen3 revproxy surface, so the
CLI does not expose them.
