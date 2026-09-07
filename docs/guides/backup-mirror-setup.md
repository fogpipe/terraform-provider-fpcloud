
---
page_title: "Off-site database backups — bring your own bucket"
---

# Off-site database backups — bring your own bucket

Every managed database backs itself up into the platform's own store on a
schedule you set, with continuous WAL archiving behind it
([database-backups](database-backups.md)). This guide is about the **second
copy**: a logical dump of the database, written straight into a bucket **you**
own, on a schedule of its own. It is opt-in, per database, and it changes
nothing about the platform's backups — they stay the point-in-time path.

What lands in your bucket is a `pg_dump -Fc` of the database, taken by a Job the
platform runs and uploaded with `rclone`. No credential of yours is stored for
the keyless provider: the platform is an OIDC issuer, and the Job presents a
short-lived token that your cloud trusts for exactly one database. A dump is a
whole copy at the moment it ran, so the recovery point of an external backup is
its schedule — there is no WAL and no point-in-time restore from it.

## Which provider

| Provider | Credential | Use it for |
|---|---|---|
| `aws` | **keyless** — an IAM role your account lets the platform's identity assume | An S3 bucket in your AWS account |
| `s3` | a static access key + secret, stored encrypted and never returned | Any S3-compatible store: Cloudflare R2, Backblaze B2, Hetzner Object Storage, Garage, a GCS bucket through its S3-compatible endpoint |
| `gcp` | **keyless** — Workload Identity Federation, impersonating a service account you name | A Google Cloud Storage bucket in your project |

## 1. Set the destination

One destination per database; setting it again replaces it. `--schedule` is a
5-field cron expression; leave it out for a destination you only ever run by
hand.

```bash
# keyless AWS
fpcloud db backup destination set mydb \
  --provider aws \
  --bucket my-db-backups \
  --region eu-north-1 \
  --role-arn arn:aws:iam::123456789012:role/fpcloud-backup \
  --schedule '0 4 * * *'

# static key: Cloudflare R2 (endpoint = https://<account-id>.r2.cloudflarestorage.com, region "auto")
fpcloud db backup destination set mydb \
  --provider s3 \
  --endpoint https://<account-id>.r2.cloudflarestorage.com \
  --region auto \
  --bucket my-db-backups \
  --access-key-id <R2_ACCESS_KEY_ID> \
  --secret-access-key <R2_SECRET_ACCESS_KEY> \
  --schedule '0 4 * * *'

# keyless GCP
fpcloud db backup destination set mydb \
  --provider gcp \
  --bucket my-db-backups \
  --wif-provider projects/123456789/locations/global/workloadIdentityPools/fpcloud/providers/fpcloud-oidc \
  --service-account fpcloud-backup@my-project.iam.gserviceaccount.com \
  --schedule '0 4 * * *'

fpcloud db backup destination show mydb    # the config, the last run, never the secret
fpcloud db backup destination unset mydb   # stop; the bucket and what is in it are yours and untouched
```

`--secret-access-key` is write-only. Leave it out on a later `set` — to change
the schedule, say — and the stored one is kept.

The same destination in OpenTofu, with the
[`fpcloud_database_backup_destination`](cli-and-terraform.md#opentofu-provider--fogpipefpcloud)
resource:

```hcl
resource "fpcloud_database_backup_destination" "offsite" {
  database_id   = fpcloud_database.main.id
  provider_type = "aws"
  bucket        = "my-db-backups"
  region        = "eu-north-1"
  role_arn      = "arn:aws:iam::123456789012:role/fpcloud-backup"
  schedule      = "0 4 * * *"
}

resource "fpcloud_database_backup_destination" "gcs" {
  database_id     = fpcloud_database.main.id
  provider_type   = "gcp"
  bucket          = "my-db-backups"
  wif_provider    = "projects/123456789/locations/global/workloadIdentityPools/fpcloud/providers/fpcloud-oidc"
  service_account = "fpcloud-backup@my-project.iam.gserviceaccount.com"
  schedule        = "0 4 * * *"
}

resource "fpcloud_database_backup_destination" "r2" {
  database_id       = fpcloud_database.main.id
  provider_type     = "s3"
  endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
  region            = "auto"
  bucket            = "my-db-backups"
  access_key_id     = var.r2_access_key_id
  secret_access_key = var.r2_secret_access_key
  schedule          = "0 4 * * *"
  flat_layout       = true
}
```

In the console, the same form is the **Backup destination (bring your own
bucket)** section of a database's page, with the last run beside it.

## 2. Let the platform in (AWS and GCP)

The `s3` provider needs nothing here: the key is the credential. For `aws` and
`gcp`, your account has to trust the identity the platform presents. It is a JWT
from the issuer `https://oidc.cloud.fogpipe.com` whose subject names exactly one
database:

```
backup:<org short id>/<project>/<database>
```

Your org's short id is the `ID` column of `fpcloud org list`, and
`fpcloud db backup create mydb --external` prints the full subject as
`Identity`. Pin the trust policy to it — or to `backup:<org short id>/<project>/*`
to cover every database in a project.

```bash
ISSUER=oidc.cloud.fogpipe.com
ACCT=123456789012
BUCKET=my-db-backups
SUB='backup:k3f/prod/mydb'

# once per AWS account: register the issuer
aws iam create-open-id-connect-provider --url "https://$ISSUER" \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 0000000000000000000000000000000000000000

# a role only that database's token can assume
cat > trust.json <<JSON
{ "Version": "2012-10-17", "Statement": [{
    "Effect": "Allow",
    "Principal": { "Federated": "arn:aws:iam::${ACCT}:oidc-provider/${ISSUER}" },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": { "StringEquals": {
      "${ISSUER}:aud": "sts.amazonaws.com",
      "${ISSUER}:sub": "${SUB}" } } }] }
JSON
aws iam create-role --role-name fpcloud-backup --assume-role-policy-document file://trust.json

# and only that bucket
aws iam put-role-policy --role-name fpcloud-backup --policy-name bucket --policy-document "{
  \"Version\": \"2012-10-17\", \"Statement\": [{ \"Effect\": \"Allow\",
  \"Action\": [\"s3:PutObject\", \"s3:GetObject\", \"s3:ListBucket\"],
  \"Resource\": [\"arn:aws:s3:::${BUCKET}\", \"arn:aws:s3:::${BUCKET}/*\"] }] }"
```

The role's ARN is what `--role-arn` / `role_arn` takes. The thumbprint is a
formality — STS fetches the issuer's keys over TLS. If a run fails on the
exchange, compare the `Identity` the CLI printed against the policy's `sub`,
byte for byte: that and the audience are the only two things it checks.

### GCP

Google's side is a workload-identity pool that trusts the same issuer, and a
service account the pool's principal may impersonate. The backup writes as that
service account, so the bucket grant goes on it and nothing else changes.

```bash
ISSUER=https://oidc.cloud.fogpipe.com
PROJECT=my-project
POOL=fpcloud
PROVIDER=fpcloud-oidc
BUCKET=my-db-backups
SUB='backup:k3f/prod/mydb'
SA="fpcloud-backup@${PROJECT}.iam.gserviceaccount.com"

# once per project: a pool, and a provider that trusts the issuer
gcloud iam workload-identity-pools create "$POOL" --location=global \
  --project="$PROJECT" --display-name="Fogpipe Cloud"
gcloud iam workload-identity-pools providers create-oidc "$PROVIDER" \
  --location=global --project="$PROJECT" --workload-identity-pool="$POOL" \
  --issuer-uri="$ISSUER" \
  --attribute-mapping="google.subject=assertion.sub" \
  --attribute-condition="assertion.sub == \"${SUB}\""

# the identity the backup writes as, and the bucket it may write to
gcloud iam service-accounts create fpcloud-backup --project="$PROJECT"
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET}" \
  --member="serviceAccount:${SA}" --role=roles/storage.objectAdmin

# let exactly that subject impersonate it
POOL_ID="$(gcloud iam workload-identity-pools describe "$POOL" --location=global \
  --project="$PROJECT" --format='value(name)')"
gcloud iam service-accounts add-iam-policy-binding "$SA" --project="$PROJECT" \
  --role=roles/iam.workloadIdentityUser \
  --member="principal://iam.googleapis.com/${POOL_ID}/subject/${SUB}"

# what --wif-provider takes
gcloud iam workload-identity-pools providers describe "$PROVIDER" --location=global \
  --project="$PROJECT" --workload-identity-pool="$POOL" --format='value(name)'
```

`--wif-provider` takes that resource path
(`projects/<number>/locations/global/workloadIdentityPools/<pool>/providers/<provider>`)
and `--service-account` the service-account email. The attribute condition is
what pins the trust to one database; widen it to
`assertion.sub.startsWith("backup:k3f/prod/")` to cover a project, and bind
`principalSet://…/attribute.../…` instead of `principal://…/subject/…` if you do.

The platform exchanges the token at `sts.googleapis.com` and then impersonates
the service account, so a failure names which half refused: `invalid_target`
means the pool or provider path is wrong, an `unauthorized_client` or a subject
mismatch means the attribute condition does not admit the `Identity` the CLI
printed, and a `403` on the object means the bucket binding is missing.

**The bucket must already exist.** The backup never creates or probes it — the
grant above is deliberately object-level, and a bucket check would ask for a
permission it does not include.

## 3. Run it, and read the result

```bash
fpcloud db backup create mydb --external     # dump → upload, right now
fpcloud db backup destination show mydb      # last run: when, and succeeded / failed / running
```

The scheduled run is the same Job on the cron you set. A failed run is reported
on `destination show`, on the database's page in the console and as
`last_run_status` on the provider resource; the platform's own backups are
reported separately and are not affected by anything that happens here.

A destination with a schedule is also part of the platform's **restore drill**:
periodically the latest dump in your bucket is restored over a scratch copy of
your database and asked a question, the same way the platform's own archives
are. A dump that uploaded cleanly and does not `pg_restore` — a truncated
object, an archive a client older than your server wrote — is found there
rather than on the day you reach for it. The outcome is yours to read:
`destination show` prints **Restore Proved** — when your dump was last
restored and answered a query, or what the latest attempt failed with — the
console's destination panel says the same, and the provider resource carries
it as `last_restored_at`, `last_restore_attempt_at` and `last_restore_error`.
It is a reading, not a button: the drill runs on the platform's own rotation,
one database per interval. On-demand-only destinations are not drilled: a dump
you took by hand is one you can test by hand, and the reading says so.

## What is in your bucket

By default dumps are written under `<prefix>/<project>/<database>/`, where
`<prefix>` is whatever you set with `--prefix` / `prefix` and is left out
entirely when unset. Each run writes `dump-<timestamp>.pgdump` and overwrites
`latest.pgdump`. Nothing is deleted: retention in your bucket is your bucket's
lifecycle rule.

If you would rather the platform not impose its project/database nesting on a
bucket you own outright, set `--flat-layout` / `flat_layout = true`: objects
then land at `<prefix>/`, or the bucket root when `prefix` is unset too.

`bucket`, `prefix` and the object name passed to `db restore --from` may
contain only letters, digits, `.`, `_`, `-` and `/`, and must start with a
letter or digit.

## Restoring from your bucket

```bash
fpcloud db restore mydb --external                          # latest.pgdump
fpcloud db restore mydb --external --from dump-20260901-040000.pgdump
```

`--external` **overwrites `mydb` in place** — the dump is restored over the
live database with `pg_restore --clean --if-exists --no-owner`, and what it
holds now is gone. That is the opposite of a restore from the platform's own
backups, which always builds a new database. It asks for confirmation first
(`--yes` skips the prompt), and it takes neither `--target` nor
`--point-in-time`: there is one dump and it lands on top of the source, so
both are refused rather than ignored.

To land a dump somewhere safe first, restore it into a database you own outside
the platform — the objects are ordinary `pg_dump` custom-format archives:

```bash
pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" latest.pgdump
```

## Reference

| Thing | Value |
|---|---|
| Issuer / `iss` | `https://oidc.cloud.fogpipe.com` |
| Discovery | `https://oidc.cloud.fogpipe.com/.well-known/openid-configuration` |
| JWKS | `https://oidc.cloud.fogpipe.com/openid/v1/jwks` |
| `sub` | `backup:<org short id>/<project>/<database>` |
| `aud` (AWS) | `sts.amazonaws.com` |
| Dump format | `pg_dump -Fc`, taken by a client on the database's own Postgres major |
| Objects | `dump-<timestamp>.pgdump` per run, `latest.pgdump` overwritten |
| API | `GET/PUT/DELETE /api/v1/databases/{id}/backup-destination`, `POST …/backup-destination/sync`, `POST …/backup-destination/restore` |
