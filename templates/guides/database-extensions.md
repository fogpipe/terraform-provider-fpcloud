
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

## Where an extension lives

An extension installs into `public` unless you say otherwise:

```bash
fpcloud db update events --extension vector:extensions \
  --cpu 500m --memory 1Gi --storage 10Gi
```

The schema is created if it is not there, and what the extension puts in it is
yours to use, the same as in `public`.

This matters for a dump from somewhere that used a different layout — Supabase
installs every extension into a schema called `extensions`, so its dump
references `extensions.vector` by name. Asking for the extension in that schema
means the dump loads unedited ([migrating from
Supabase](migrating-from-supabase.md)).

Choose it at install: the platform installs an untrusted extension as a
superuser you never hold, so moving one afterwards is not something your role
can do. Changing the schema of an extension you already have means removing it
and asking for it again, which drops what it created.

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

## Removing one

An extension you leave out of the set is **uninstalled**, not just unmounted:
the platform runs the `drop extension` you cannot, and only then takes the
image away.

```bash
fpcloud db update events --extension semver \
  --cpu 500m --memory 1Gi --storage 10Gi     # pg_statecharts is dropped
```

`drop extension` takes what the extension created along with it — for
`pg_statecharts`, the `fsm` schema and every state machine in it. The CLI asks
before it does that (`--yes` skips the prompt), and the console asks in the same
place; Terraform does not ask at all, so a removed element of `extensions` is
applied as written.

**Anything of yours that depends on it refuses the removal.** A column typed on
the extension's type, an index using its operator class, a view calling its
function: the removal is refused and the objects are named, so drop them
yourself and remove the extension afterwards. Nothing is ever cascaded away on
your behalf.

If the drop fails, nothing is unmounted — the database is left exactly as it
was, with the extension installed and working.

## What is available

| Extension | Version | Provides |
|-----------|---------|----------|
| `semver` | 0.41.0 | A semantic-version type with comparison operators and indexing |
| `pg_statecharts` | 0.0.0 | State machines in SQL, interpreted from statechart definitions |
| `vector` | 0.8.6 | Vector similarity search (pgvector): the `vector` type, distance operators, HNSW and IVFFlat indexes |
| `pgaudit` | 18.1 | Per-statement audit logging of `fpcloud db connect` sessions, attributed to the person who opened them |

Asking for anything else is refused at create, with the list of what is
available — a database never reports an extension it does not have. Dependencies
come along automatically: `pg_statecharts` needs `ltree`, and you do not have to
name it.

Some of these are already inside the database image — `vector` is — and still
need asking for: what the platform does for you is the `CREATE EXTENSION` that
superuser gates, whether or not the files are on disk. The platform installs a
curated extension into the `public` schema.

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

## Auditing what a person did through a tunnel

`pgaudit` is curated like the rest and enabled the same way:

```bash
fpcloud db update mydb --extension pgaudit \
  --cpu 500m --memory 1Gi --storage 10Gi
```

It records the statements run through `fpcloud db connect` — reads and writes,
attributed to the fpcloud identity that opened the session, because a tunnel
authenticates as a role minted for that person rather than as your database's
shared role.

**Your application's own traffic is never audited.** The filter is attached to
the session roles the platform mints, not to the database, so everything
connecting as your `app` role — which is every app, job and migration you run —
is untouched. That is a property of how it is switched on, not a setting you
could get wrong.

Enabling it is refused on a database whose sessions cannot be attributed to a
person yet. Such a database still hands out the shared credential, so the
audit log would record that `app` read a table and tell you nothing; the
refusal names what has to happen first rather than turning something on that
answers nothing.

Entries are kept for as long as your logs are (14 days). The platform can say
who connected to a database for as long as the audit log exists, and what they
did for a fortnight.

## What it costs

Adding or removing an extension is a **restart-class** change: the database
rolls its pods to pick up the mount change, or the library `pgaudit` is loaded
through, the same as a version or resource
change. Everything else about the database is unchanged — backups, restore,
point-in-time recovery and failover all behave exactly as they do without
extensions, because the database itself is still the image we operate.

A restored database inherits the extensions of the one it was restored from, so
a restore does not land a schema whose objects have nothing to bind to.
