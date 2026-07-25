
---
page_title: "Terraform / OpenTofu remote state in an fpcloud bucket"
---

# Terraform / OpenTofu remote state in an fpcloud bucket

Keep your Terraform/OpenTofu state in a managed fpcloud bucket via the standard
`s3` backend — EU-sovereign, no external AWS/R2 dependency, with **state locking**
so concurrent `apply`s can't corrupt state.

## 1. Create a bucket and a scoped key

```bash
fpcloud storage bucket create tfstate
fpcloud storage keys create tfstate --name terraform --read --write
```

`keys create` prints the `access_key_id` and (once) the `secret_access_key`.
The bucket's global S3 name is `<project>-<name>` — check it with:

```bash
fpcloud storage bucket get-credentials tfstate   # shows bucket name, endpoint, region
```

## 2. Configure the backend

```hcl
terraform {
  backend "s3" {
    bucket = "myproject-tfstate"                  # <project>-<name>
    key    = "env/prod/terraform.tfstate"
    region = "garage"                             # must be "garage" (Garage's SigV4 region)

    endpoints = { s3 = "https://s3.cloud.fogpipe.com" }

    # Non-AWS S3 essentials
    use_path_style              = true
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_region_validation      = true

    # State locking — safe on fpcloud (see "Locking" below)
    use_lockfile = true
  }
}
```

Supply the key via the standard AWS env vars:

```bash
export AWS_ACCESS_KEY_ID=GK...
export AWS_SECRET_ACCESS_KEY=...
tofu init
tofu apply
```

## Locking

`use_lockfile = true` gives you native state locking — a second concurrent
`apply` is blocked until the first releases, the same as AWS S3 or GCS. No
DynamoDB or any extra resource is required.

This works because `s3.cloud.fogpipe.com` runs a conditional-write shim in front
of the object store that supplies the atomic compare-and-set the lock needs.
**Locking only works through `s3.cloud.fogpipe.com`.** If you point the
backend at a raw store endpoint that bypasses it, `use_lockfile` will appear to
work but won't actually enforce mutual exclusion — always use
`https://s3.cloud.fogpipe.com`.

## Notes

- **Region must be `garage`** — it has to match the store's SigV4 region even
  with `skip_region_validation`.
- **`use_path_style = true`** is required; the store is path-style, not
  vhost-style.
- Scope keys per state bucket with `--read --write`; a key can't reach other
  projects' buckets.
