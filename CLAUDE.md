# OpenTofu provider for Fogpipe Cloud

The Terraform/OpenTofu provider. Public, Apache-2.0.

## What this repo is

- `internal/provider/` — one `resource_*.go` or `datasource_*.go` per API
  resource, plus the provider wiring. Every file maps client structs onto a
  terraform-plugin-framework schema by hand.
- `templates/` — the hand-written pages tfplugindocs renders from. `docs/` is
  **output**: `just docs` rewrites that tree from scratch, so a page edited
  there is gone on the next run.
- `examples/` — the HCL tfplugindocs embeds into the generated pages.

It talks to the API only through `github.com/fogpipe/cloud-cli/pkg/client`,
the same module the platform server imports. There is no second HTTP client and
no vendored copy — a client change arrives here as a version bump.

## It is public

Nothing operator-internal goes here. Operator-only API paths live under
`/api/v1/admin/*` and no resource may expose them.

## Every API resource maps 1:1, and CI holds you to it

The mirror is not a convention, it is gated — in this repo's own CI
(`scripts/tf-resource-coverage.ts`, `scripts/tf-field-coverage.ts`):

- a client method no resource or data source calls fails
  `tf-resource-coverage.ts`
- a client field no schema attribute expresses fails `tf-field-coverage.ts`,
  which pairs client type `X` with this repo's `XResourceModel` one level into
  nested objects

Both resolve `github.com/fogpipe/cloud-cli` at its **latest release**, not the
version this repo's `go.mod` pins — judging the provider against its own pin
would make the gate self-satisfying, staying green while the pin sits releases
behind. A red build here after a `cloud-cli` release is the signal to bump.

`templates/guides` drifting from the platform's docs pool fails a check in
platform's CI instead, which still clones this repo to run it — that gate
reads platform's private docs pool, so it cannot move here.

Both coverage gates accept reviewed exclusions via baselines in this repo
(`scripts/tf-resource-coverage-baseline.txt`, `scripts/tf-field-coverage-baseline.txt`).
Baselining is a decision, not a way past a red build.

Nested structures are `ListNestedAttribute` / `SingleNestedAttribute`, never
`Blocks:`. Match what is already there.

## A change here rarely stands alone

A new capability is usually three repos in order: `cloud-cli` (change
`pkg/client`, land **and tag** it), then the platform, then this repo's schema
on top. Nothing here can move before that tag exists — a module dependency
resolves a version, not a branch.

`just tree <name>` at the workspace root puts this repo, the CLI and the
platform under one `go.work`, so a `pkg/client` change is testable here before
it is tagged. Without it you are building against the last release.

See `../docs/provider-sync.md` for the full model.

## Releases are not cut here

This repo, the CLI and the platform carry **one shared version**, pushed to all
three at once by `just release vX.Y.Z` at the workspace root
(`/release-semver`). So `vX.Y.Z` of the provider is built against exactly that
release of the API, and the version pinned in `required_providers` names a known
server.

Tagging this repo alone is what let the three numbers drift apart before. It is
also the one repo whose release is nobody's deploy — the platform ships when it
is deployed and the CLI when it is tagged, so an untagged `main` here looks
finished from inside every other repo and reaches no tenant.

Regenerate docs (`just docs`) and commit them before a release if the schema
moved; GoReleaser signs and publishes on the tag, and the registry refuses an
unsigned version.

## Testing

`just test` is unit-only. `just testacc` runs the acceptance suite against a
**live** API and needs `FPCLOUD_API_KEY` (plus `FPCLOUD_API_URL` for anything
but production). Each test randomizes resource names and asserts `CheckDestroy`,
so a failed run leaves nothing behind. CI runs them on manual dispatch and
weekly, never on a PR — they cost real resources.

## Conventions

- `go build ./...`, `go test ./...`, `gofmt` — CI gates all three.
- Dependency versions track the platform's, `pkg/client` above all. A bare
  `go mod tidy` resolves to latest and drags the server's graph forward through
  this module.
