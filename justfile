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

# Regenerate docs/ from schema + examples/ + templates/. Run before tagging a
# release. Everything under docs/ is output — this rewrites the tree from
# scratch, so hand-written pages go in templates/ (see templates/guides).
docs:
    tfplugindocs generate --provider-name fpcloud

# Local GoReleaser dry-run (no publish, no signing).
snapshot:
    goreleaser release --snapshot --clean
