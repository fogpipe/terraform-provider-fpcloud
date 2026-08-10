
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

## Connect your GitHub account

Once per project, connect the GitHub account your runners will serve:

```bash
fpcloud github connect
```

This opens GitHub, installs the **Fogpipe** app on the account you choose, and
records it against this project. That is the entire setup: nothing to copy, no
key to handle, and no organization name to type.

You are asked to authorize the install as yourself, and only accounts **you can
administer** are offered. Fogpipe never takes an organization name on trust, so
no project can point runners at an account it does not control.

```bash
fpcloud github status      # which account this project is connected to
fpcloud github disconnect  # remove the binding (delete its pools first)
```

## Create a pool

A pool needs nothing but a name — it serves every repository in the connected
account:

```bash
fpcloud runner create ci
```

Then use the pool from a workflow. Its name is its `runs-on` label:

```yaml
jobs:
  test:
    runs-on: ci
    steps:
      - uses: actions/checkout@v5
      - run: make test
```

## Bringing your own credential

The Fogpipe app is the default and needs nothing from you. Two cases it cannot
serve, both selected explicitly:

These need `--github-account`, because a key or a token says nothing about which
account it is for. Holding the credential is itself the proof it is yours:

```bash
# your own GitHub App — for an organization whose policy forbids third-party apps
fpcloud runner create ci --credential app --github-account acme \
    --github-app-id 123456 \
    --github-app-installation-id 7891011 \
    --github-app-private-key-file ./acme-ci.private-key.pem

# a personal access token — fine for a first try
fpcloud runner create ci --credential token --github-account acme --github-token ghp_…
```

Your own app needs the **Self-hosted runners: Read & write** organization
permission. A token carries a person's full access and dies with their account,
so it is not something to leave in place.

Anything secret you supply is encrypted on arrival and **write-only**: it is
never returned by the API, the CLI or the console. Rotate it by supplying a new
one:

```bash
fpcloud runner update ci --credential app --github-app-private-key-file ./new-key.pem
```

A credential is replaced whole. A pool authenticates as an app *or* as a token,
never as half of each.

## Scaling

```bash
fpcloud runner create ci --min 1 --max 4 ...
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
fpcloud runner create ci --builds ...
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

## Where runners run

Runner pods get a namespace of their own, alongside your project's rather than
inside it. It is per-project — no other tenant's jobs share it — and it exists
so the credential a pool registers with is somewhere nothing you run can read,
including the code in your own workflows.

Two things follow from that. Runner pods are allowed outbound HTTPS, so they
reach GitHub, actions and toolchains without you opening your project's egress.
And they **cannot** reach your project's own services by their in-cluster names:
a job that needs your database or your app should go through its public
address, or bring its own service container.

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
