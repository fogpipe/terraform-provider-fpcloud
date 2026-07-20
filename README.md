# terraform-provider-fpcloud

The OpenTofu / Terraform provider for [Fogpipe](https://cloud.fogpipe.com) — a
European PaaS. Manage projects, apps, databases, domains, IAM, service
accounts, and OIDC federation declaratively.

The provider is a 1:1 mirror of the `fpcloud` CLI and talks to the same REST API,
so anything you can do in one you can do in the other.

## Usage

```hcl
terraform {
  required_providers {
    fpcloud = {
      source  = "fogpipe/fpcloud"
      version = "~> 0.1"
    }
  }
}

provider "fpcloud" {
  # api_key = "fp-..."                          # or FPCLOUD_API_KEY (recommended)
  # api_url = "https://api.cloud.fogpipe.com"   # or FPCLOUD_API_URL
}

resource "fpcloud_project" "web" {
  name = "web"
}

resource "fpcloud_app" "api" {
  project = fpcloud_project.web.id
  name    = "api"
  image   = "nginx:latest"
}
```

Set the API key out of band — `export FPCLOUD_API_KEY=fp-...` — rather than in HCL.

Credentials resolve in order: provider block → `FPCLOUD_API_KEY`/`FPCLOUD_API_URL` env → an API key in the fpcloud CLI config (`~/.fpcloud/config.yaml`, honouring `FPCLOUD_CONFIG_DIR`) → the Google OIDC login via `fpcloud get-token`. So after **any** CLI login — `fpcloud login` (Google) or `fpcloud auth login` (API key) — a bare `tofu apply` just works, with nothing in HCL or env. This is the AWS/GCP model: the CLI login doubles as the provider's default credentials, and the OIDC path delegates token refresh to the CLI exactly like a kubectl exec plugin (so the `fpcloud` binary must be on `PATH`). Prefer the env var in CI (minted by OIDC federation); the CLI fallback is for local, interactive use.

See [`examples/`](./examples) for per-resource usage.

## Development

A Nix flake provides the toolchain (Go, just, goreleaser, tfplugindocs, opentofu, gnupg):

```bash
nix develop        # or: direnv allow

just build         # compile
just test          # unit tests
just testacc       # acceptance tests (needs FPCLOUD_API_KEY against a live API)
just docs          # regenerate docs/ from schema + examples
just snapshot      # local GoReleaser dry-run (no publish)
```

To run a local build against a config, use a `dev_overrides` block in
`~/.terraformrc` pointing `fogpipe/fpcloud` at your `$GOBIN`.

## Acceptance tests

The `TestAcc*` tests in `internal/provider/` are **acceptance tests**: they drive
the real provider against a **live fpcloud API**, creating and destroying real
projects, apps, and buckets. They exist to catch "Provider produced inconsistent
result after apply" regressions (two have shipped: the app `tier`→`mode` rename
and `fpcloud_bucket` quota defaults).

They only run when `TF_ACC=1` is set — a plain `go test ./...` skips them, so
unit CI stays offline-safe. **Warning: running them mutates a live API.** Point
them at a throwaway org, never production.

```bash
TF_ACC=1 \
  FPCLOUD_API_URL=https://api.cloud.fogpipe.com \
  FPCLOUD_API_KEY=fp-... \
  go test ./internal/provider -run TestAcc -v -timeout 30m
# or, via the flake toolchain:
just testacc
```

Each test randomizes resource names and asserts a `CheckDestroy` so a failed run
does not leave resources behind. In CI they run only on manual dispatch (and a
weekly schedule) via `.github/workflows/acceptance.yml`, which reads
`secrets.FPCLOUD_API_KEY` and `vars.FPCLOUD_API_URL`.

## Releasing

Releases are cut by GoReleaser in CI on a `v*` tag. The registry requires signed
artifacts, so the repo needs two secrets:

- `GPG_PRIVATE_KEY` — ASCII-armored private key: `gpg --armor --export-secret-keys <KEY_ID>`
- `PASSPHRASE` — that key's passphrase

Register the **public** half of the key with the OpenTofu Registry (and the
Terraform Registry, if publishing there). Then:

```bash
just docs && git commit -am "docs" # if schema changed
git tag v0.1.0
git push origin v0.1.0
```

## License

[MPL-2.0](./LICENSE)
