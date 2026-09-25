
---
page_title: "Templates — deploy a self-hosted app in one command"
---

# Templates — deploy a self-hosted app in one command

The catalog is a curated set of self-hosted apps — web analytics, dashboards,
team chat, newsletters, cron monitoring — each deployed in one command or one
click as **an app, a managed Postgres and a bucket that your project then owns
outright**. There is no installation object and nothing to uninstall: what a
template makes is exactly what `app create`, `db create` and `bucket create`
make, and it is resized, reconfigured, backed up, billed and deleted through
the same commands as anything you built yourself.

```bash
fpcloud template list
fpcloud template get grafana
fpcloud template deploy grafana --name grafana --input GF_SECURITY_ADMIN_PASSWORD='…'
```

The console has the same catalog under **Catalog**, with a form per entry.

## What an entry is

Every entry is a reviewed definition: the upstream image pinned to a digest,
the version it carries, the port, a health check, a recommended size, the
environment the app needs and what it needs beside it. The image the app runs
is the platform's own mirror of that digest, so it is scanned, digest-pinned
and retention-protected like every other image on the platform — `fpcloud app
cves` answers for it the way it does for your own images.

A template app runs as a non-root user like any other app on the platform.
Where the upstream image declares no user, the entry names the uid the app is
created with and `fpcloud template get` shows it as **Runs as**; an entry whose
first boot takes longer than the shared health check allows — a JVM running its
migrations — carries the startup probe that gives it that time, shown as
**Startup**. Both are ordinary app settings afterwards, read and changed with
`fpcloud app get` and `fpcloud app set-probes`.

An entry's environment is wired for you at deploy: the database it creates is
seeded into the app's env as `DATABASE_URL` and the libpq variables, the bucket
arrives as `S3_*`, and the app's own address as `FPCLOUD_APP_URL` — so nothing
is copied by hand. The database credential in env is the template's wiring, not
a recommendation; your own apps mount the database's owner secret as a file
instead.

`fpcloud template get <name>` shows the inputs an entry asks for — an admin
password, an SMTP relay — and the note that says what to do after the deploy
where an app takes a setting only in its own UI.

## The database and the bucket are yours

The database is `<name>-db`, the bucket `<name>`. They appear in `fpcloud db
list` and `fpcloud bucket list` like any other, with the same backups, the same
restore drill and the same quotas. Deleting the app does not delete them, for
the same reason deleting any app does not: the data is the part worth keeping.

## Updates

An app remembers its template. When the catalog carries a newer version than
the release the app is on, `fpcloud app get` reports it as `template_update`
and the console offers it on the app's page:

```bash
fpcloud app upgrade grafana
```

That is an ordinary deploy of the newer mirror, with the version as its release
name — so `fpcloud app rollback <version>` takes you back, and `fpcloud app
version` says where you are. Any configuration key the new version adds is
seeded; a key you have set is left as it is.

Deploying an image of your own onto a template app makes it yours from then
on: the catalog stops offering upgrades it can no longer reason about, and the
app is one you built.

## What the catalog admits

An entry is listed only when the whole of its state lives in a managed
Postgres, a bucket, or both, it runs as one HTTP container, it needs no Redis,
and its licence permits a hosting provider to offer it. That is what makes one
click honest: an app that fits has nothing to back up beyond what the platform
already backs up, and nothing that pins it to a machine. It is also why some
popular names are absent — anything on SQLite, anything that keeps its
repositories or attachments on a disk, anything that will not start without
Redis.
