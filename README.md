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
