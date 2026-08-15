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
#
# The schema is exported here rather than left to tfplugindocs, which otherwise
# downloads a Terraform binary and verifies its signature against a HashiCorp
# PGP key that has expired — it fails for everyone, not just where terraform is
# absent. Exporting through the tofu already on PATH avoids the download; the
# provider address is rewritten because tofu reports registry.opentofu.org and
# tfplugindocs looks the schema up under the bare provider name.
#
# tfplugindocs itself is pinned and fetched here rather than expected on PATH,
# so this recipe runs from a clean checkout and everyone generates docs with the
# same version — a different one reflows every page and buries the schema change
# in the diff.
docs:
    #!/usr/bin/env bash
    set -euo pipefail
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    go build -o "$work/terraform-provider-fpcloud" .
    cat > "$work/tofurc" <<EOF
    provider_installation {
      dev_overrides { "fogpipe/fpcloud" = "$work" }
      direct {}
    }
    EOF
    printf 'terraform {\n  required_providers {\n    fpcloud = { source = "fogpipe/fpcloud" }\n  }\n}\n' > "$work/main.tf"
    TF_CLI_CONFIG_FILE="$work/tofurc" tofu -chdir="$work" providers schema -json \
      | sed 's#registry.opentofu.org/fogpipe/fpcloud#fpcloud#' > "$work/schema.json"
    GOWORK=off go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.24.0 \
      generate --provider-name fpcloud --providers-schema "$work/schema.json"

# Local GoReleaser dry-run (no publish, no signing).
snapshot:
    goreleaser release --snapshot --clean

# Does every client method have an actual resource/data source calling it, not
# just client-layer plumbing? New gaps fail; existing ones are accepted via
# scripts/tf-resource-coverage-baseline.txt.
tf-resource-coverage *args:
    deno run --allow-read --allow-write --allow-run --allow-env scripts/tf-resource-coverage.ts {{args}}

# And once a resource calls the method, can Terraform reach every field of it?
# tf-resource-coverage is satisfied forever by the first call, so a field added
# to pkg/client stayed unsettable in HCL with every gate green. New gaps fail;
# reviewed ones are accepted via scripts/tf-field-coverage-baseline.txt.
tf-field-coverage *args:
    deno run --allow-read --allow-write --allow-run --allow-env scripts/tf-field-coverage.ts {{args}}
