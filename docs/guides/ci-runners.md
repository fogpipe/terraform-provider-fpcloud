
---
page_title: "GitHub Actions Runners"
---

# GitHub Actions Runners

A **runner** is a pool, not a machine. Nothing runs until GitHub has a job for
it: the platform starts a pod, that pod serves exactly one job, and it is
destroyed when the job ends. An idle pool costs nothing, and no state carries
from one job to the next.

Runners live in your project's namespace, beside your apps and databases, and
are isolated from other tenants exactly the way those are.

## Create a pool

Point the pool at what it serves — one repository, or a whole organization:

```bash
# every workflow in one repository
fpcloud runner create ci --repo acme/api \
    --github-app-id 123456 \
    --github-app-installation-id 7891011 \
    --github-app-private-key-file ./acme-ci.private-key.pem

# every workflow in the organization
fpcloud runner create shared --org acme \
    --github-app-id 123456 \
    --github-app-installation-id 7891011 \
    --github-app-private-key-file ./acme-ci.private-key.pem
```

Then use it from a workflow. The pool's name is its `runs-on` label:

```yaml
jobs:
  test:
    runs-on: ci
    steps:
      - uses: actions/checkout@v5
      - run: make test
```

## The credential

A pool has to authenticate to GitHub to receive work. Two ways:

- **A GitHub App** — the one to use. Create it on your organization with the
  **Self-hosted runners: Read & write** organization permission, install it, and
  give the pool its app id, its installation id, and the private key GitHub
  shows you once.
- **A personal access token** (`--github-token`) — fine for a first try. It
  carries a person's full access and dies with their account, so it is not
  something to leave in place.

The secret half is encrypted on arrival and **write-only**: it is never returned
by the API, the CLI or the console. Rotate it by supplying a new one:

```bash
fpcloud runner update ci --github-app-private-key-file ./new-key.pem
```

Supplying a credential replaces the previous one whole. A pool authenticates as
an App *or* as a token, never as half of each.

## Scaling

```bash
fpcloud runner create ci --repo acme/api --min 1 --max 4 ...
```

- `--max` is how many jobs the pool runs at once. Jobs beyond it queue on
  GitHub. Runners share your project's CPU and memory quota with everything else
  in it, so this is a budget, not a throughput dial.
- `--min` keeps that many pods idle and ready, trading cost for a faster start.
  The default is `0` — the pool scales to zero, and a job waits a few seconds for
  its pod.
- `--cpu` and `--memory` bound one runner pod (for example `--cpu 2 --memory
  4Gi`).

## Building container images

There is no Docker daemon in a runner, and Docker-in-Docker is not available:
it needs a privileged container, which the platform does not run for anyone.
Instead, ask for a builder:

```bash
fpcloud runner create ci --repo acme/api --builds ...
```

`--builds` puts a rootless [BuildKit](https://github.com/moby/buildkit)
alongside each job and points `BUILDKIT_HOST` at it. It is reachable only from
inside that job's own pod. `docker/build-push-action` works against it through
buildx's remote driver:

```yaml
- uses: docker/setup-buildx-action@v3
  with:
    driver: remote
    endpoint: ${{ env.BUILDKIT_HOST }}
- uses: docker/build-push-action@v6
  with:
    push: true
    tags: registry.cloud.fogpipe.com/acme/api:${{ github.sha }}
```

Because the pod is per-job, no build cache is shared between jobs — every build
starts cold.

## Pushing to your Fogpipe registry

A runner is an ordinary workload in your project, so it authenticates to the
Fogpipe registry the same way your CI does anywhere else: through OIDC
federation, with no stored credential. See
[Deploy from GitHub](deploy-from-github.md).

## Watching a pool

```bash
fpcloud runner list
fpcloud runner show ci
```

`STATUS` is the pool's own state, not a job's. `pending` means the platform has
declared the pool and is waiting for it to register with GitHub — normal for a
few seconds after creation, and worth investigating if it persists (usually a
credential without the self-hosted-runner permission).

`ACTIVE` is how many runner pods exist right now, which is how many of your jobs
are running.

## Network access

Runner pods are allowed outbound HTTPS so they can reach GitHub, download
actions and fetch toolchains — you do not need to open your project's egress to
get that. Anything else a job needs (a package registry on a non-standard port,
say) follows your project's egress setting.

## Removing a pool

```bash
fpcloud runner delete ci
```

The pool is deregistered from GitHub and its pods go with it. Workflows that
still say `runs-on: ci` will queue with nothing to pick them up, so change them
first.

## Limits

- Runners serve **your own** repositories. Scheduling workflows from forked pull
  requests is off — it would let anyone who opens a PR run code in your project.
- A pool is isolated at the same level as everything else in your project: a
  Kubernetes namespace. That is the right boundary for your own CI. It is not a
  sandbox for untrusted code, and nothing here claims to be one.
