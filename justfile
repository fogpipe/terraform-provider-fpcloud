default:
    @just --list

# Compile the provider.
build:
    go build ./...

# Unit tests.
test:
    go test ./... -count=1

# Acceptance tests hit a live fpcloud API. Requires FPCLOUD_API_KEY (+ optional
# FPCLOUD_API_URL) in the environment.
testacc:
    TF_ACC=1 go test ./internal/provider/ -v -count=1 -timeout 30m

# Regenerate docs/ from schema + examples/. Run before tagging a release.
docs:
    tfplugindocs generate --provider-name fpcloud

# Local GoReleaser dry-run (no publish, no signing).
snapshot:
    goreleaser release --snapshot --clean
