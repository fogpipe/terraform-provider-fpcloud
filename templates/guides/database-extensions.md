
---
page_title: "Database Extensions"
---

# Database Extensions

A managed Postgres database can carry extensions beyond the ones the stock image
ships. There are two kinds, and only one of them needs the platform at all.

## Trusted extensions: install them yourself

Postgres marks some extensions **trusted**, and the database owner can install
those with no special privilege. `citext`, `pgcrypto`, `ltree`, `hstore`,
`uuid-ossp` and the rest of that set are already present in the image, so they
need nothing from fpcloud:

```sql
create extension if not exists citext;
```

That works today, under the role your `DATABASE_URL` connects as.

## Untrusted extensions: ask for them at create

`CREATE EXTENSION` on an **untrusted** extension requires superuser, and a
managed database hands out no superuser — not to you, and not to anything
running in your project. So the platform installs those for you:

```bash
fpcloud db create events --version 18 --extension semver --extension pg_statecharts
```

The set is replaceable later, and the flag takes the whole set:

```bash
fpcloud db update events --extension semver --extension pg_statecharts \
  --cpu 500m --memory 1Gi --storage 10Gi
```

In Terraform:

```hcl
resource "fpcloud_database" "events" {
  project_id = fpcloud_project.app.id
  name       = "events"
  version    = "18"
  extensions = ["semver", "pg_statecharts"]
}
```

**Your migrations do not change.** Once the platform has installed an extension,
your own `create extension if not exists semver` runs as a no-op under your
ordinary role and succeeds, so a schema that already carries that line keeps
working unedited.

**What the extension creates is yours to use.** The platform installs it as a
superuser you never hold, so an extension that brings its own schema — as
`pg_statecharts` brings `fsm` — would otherwise leave you unable to read or write
a single object in it. Your role is given full rights on the schemas, tables,
sequences and functions an extension creates, as it creates them, so an upgrade
that adds a table does not need anything re-run.

Dropping one is still ours: the platform owns what it installed, so
`drop extension pg_statecharts` under your own role is refused. A migration tool
that reverts as far as its first change will stop there.

## What is available

| Extension | Version | Provides |
|-----------|---------|----------|
| `semver` | 0.41.0 | A semantic-version type with comparison operators and indexing |
| `pg_statecharts` | 0.0.0 | State machines in SQL, interpreted from statechart definitions |

Asking for anything else is refused at create, with the list of what is
available — a database never reports an extension it does not have. Dependencies
come along automatically: `pg_statecharts` needs `ltree`, and you do not have to
name it.

The catalog is curated rather than open. If you need an extension that is not
here, ask — adding one is a build we own, not something you have to package.

## Postgres 18 or later

An extension is delivered as its own image and mounted into the database, and
the mechanism that makes that possible arrived in **Postgres 18**. On 15, 16 or
17 the request is refused rather than half-applied:

```
extensions need postgres 18 or later (this database runs 17)
```

A new database is created on 18 unless you ask for something older, so this
only comes up on a database that predates that default or names an older major
explicitly. A database already running 17 does not move on its own — a major
version change rewrites the data directory and is never a side effect of asking
for an extension.

## What it costs

Adding or removing an extension is a **restart-class** change: the database
rolls its pods to pick up the new mount, the same as a version or resource
change. Everything else about the database is unchanged — backups, restore,
point-in-time recovery and failover all behave exactly as they do without
extensions, because the database itself is still the image we operate.

A restored database inherits the extensions of the one it was restored from, so
a restore does not land a schema whose objects have nothing to bind to.
