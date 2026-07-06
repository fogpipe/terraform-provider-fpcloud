PROVIDER_NAME := fpcloud

default: build

build:
	go build ./...

test:
	go test ./... -count=1

# Acceptance tests hit a live fpcloud API. Requires FPCLOUD_API_KEY (+ optional
# FPCLOUD_API_URL) in the environment.
testacc:
	TF_ACC=1 go test ./internal/provider/ -v -count=1 -timeout 30m

# Regenerate docs/ from schema + examples/. Run before tagging a release.
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate \
		--provider-name $(PROVIDER_NAME)

# Local dry-run of the release build (no publish, no signing).
snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean

.PHONY: default build test testacc docs snapshot
