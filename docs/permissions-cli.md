# Permissions CLI

`calypr-cli permissions` is the operator-facing CLI for the Arborist routes
that are actually exposed through the public Gen3 `/authz` surface.

This is intentionally not a full Arborist admin client. Raw Arborist catalog
routes like user, role, and resource CRUD are not exposed through revproxy, so
`calypr-cli` does not try to support them.

Use `calypr-cli collaborators` when a user is asking for access they do not
already have. Use `calypr-cli permissions` when an admin needs to inspect or
change the ownership and direct-access state that Arborist exposes publicly.

## Basic Shape

All commands use the normal calypr-cli profile:

```bash
calypr-cli --profile calypr permissions <command>
```

Add `--json` when you need raw output for debugging or scripts:

```bash
calypr-cli --profile calypr permissions --json auth mapping
```

## Current Auth Mapping

Show the current profile user's Arborist mapping:

```bash
calypr-cli --profile calypr permissions auth mapping
```

This command reads the public `GET /authz/mapping` surface for the current
token. It does not support arbitrary username lookups.

## Organization Membership

Organization membership is a convenience wrapper around Arborist direct-access
grants on `/programs/<org>/projects`.

```bash
calypr-cli --profile calypr permissions org-membership add user@ohsu.edu Ellrott_Lab
calypr-cli --profile calypr permissions org-membership rm user@ohsu.edu Ellrott_Lab
```

The default role is `org-member`. It carries only
`arborist/create-descendant`, which lets the member create projects under the
existing organization without granting ownership or access on existing projects.

You can specify another role when needed:

```bash
calypr-cli --profile calypr permissions org-membership add user@ohsu.edu Ellrott_Lab --role org-member
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
calypr-cli --profile calypr permissions ownership create-descendant \
  --parent /programs \
  --name Ellrott_Lab \
  --template gen3-program
```

Create a new project resource under an organization and make the caller its
owner:

```bash
calypr-cli --profile calypr permissions ownership create-descendant \
  --parent /programs/Ellrott_Lab/projects \
  --name git_drs_test \
  --template gen3-project
```

Add or remove owners:

```bash
calypr-cli --profile calypr permissions ownership add-owner \
  --resource /programs/Ellrott_Lab \
  --user user@ohsu.edu

calypr-cli --profile calypr permissions ownership rm-owner \
  --resource /programs/Ellrott_Lab \
  --user user@ohsu.edu
```

Read the normalized ownership and direct-access state for a resource:

```bash
calypr-cli --profile calypr permissions ownership get-resource \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --include-admins \
  --include-children
```

## Direct Access Power Tools

Grant or revoke direct non-owner access on an existing resource:

```bash
calypr-cli --profile calypr permissions access grant-user \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --user user@ohsu.edu \
  --role writer

calypr-cli --profile calypr permissions access revoke-user \
  --resource /programs/Ellrott_Lab/projects/git_drs_test \
  --user user@ohsu.edu \
  --role writer
```

Use these commands for ordinary direct access. Use ownership add/remove for
owner changes.

## Deliberate Non-Support

`calypr-cli permissions` does not support raw Arborist catalog/admin routes such
as:

- user CRUD
- role CRUD
- resource CRUD
- raw policy mutation
- arbitrary-user auth mapping lookup

Those routes are not part of the supported public Gen3 revproxy surface, so the
CLI does not expose them.

The legacy backend-oriented command name `calypr-cli arborist` still works as a
compatibility alias, but `permissions` is the supported user-facing name.
