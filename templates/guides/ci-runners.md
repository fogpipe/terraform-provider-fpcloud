
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

This opens GitHub and asks you to authorize as yourself. Fogpipe then records
the account against this project. That is the entire setup: nothing to copy, no
key to handle, and no organization name to type.

Only accounts **you administer** can be connected — an owner of the
organization, or your own user account. Being a member is not enough, because
connecting lets a project run CI on that account. Fogpipe never takes an
organization name on trust, so no project can point runners at an account it
does not control.

If the **Fogpipe** app is not installed on the account yet, connecting tells you
and gives you the link. If you administer more than one account with it
installed, say which:

```bash
fpcloud github connect --account acme
```

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

Then use the pool from a workflow. Its `runs-on` label is your project name and
the pool name joined — so a pool called `ci` in project `acme` is `acme-ci`:

```yaml
jobs:
  test:
    runs-on: acme-ci
    steps:
      - uses: actions/checkout@v5
      - run: make test
```

The label is scoped that way because GitHub registers runners per **account**,
and several of your projects can share one account. Without the project in the
name, two projects that both called a pool `ci` would claim the same label — and
jobs would land in whichever project's runners GitHub happened to pick.

`fpcloud runner create` prints the exact label, and `fpcloud runner show <name>`
repeats it, so you never have to assemble it yourself.

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
  GitHub. Every one of them costs cores and memory for as long as it runs, so
  this is a budget, not a throughput dial.
- `--min` keeps that many pods idle and ready, trading cost for a faster start.
  The default is `0` — the pool scales to zero, and a job waits a few seconds for
  its pod.
- `--cpu` and `--memory` bound the runner — the container your workflow's steps
  execute in (for example `--cpu 2 --memory 4Gi`). A builder, if you ask for
  one, is sized separately and adds to what the pool costs.

### Runners draw on your project's resource caps

Your pools run in a namespace of their own, and it is bounded by the same caps
as the rest of your project — **2 CPU / 4Gi / 20 pods** by default (see [project
resource caps](projects-and-access.md#project-resource-caps)). It is a second
budget of that size, not a share of the one your apps and databases spend: CI
neither takes capacity from your running services nor is starved by them.

A runner reserves a tenth of its `--cpu` and half its `--memory` against that
budget, so the default `2 CPU / 4Gi` runner reserves `200m` and `2Gi` — two of
them fit the default caps at once, whatever `--max` says. Jobs beyond what fits
wait for a free slot.

A pool too large to run even once is refused when you ask for it, rather than
accepted and left with jobs that never start:

```console
$ fpcloud runner create ci --cpu 8 --memory 32Gi
Error: a pool of this size reserves 16Gi memory, more than this project's
memory cap of 4Gi; shrink the pool or ask your operator to raise the cap
```

An existing pool that stops fitting — because its builder grew, or its project's
caps were lowered — keeps its declaration and says the same thing on
`fpcloud runner show <name>`. It is the pool you edit to fix it, so it is not
taken away from you; its runners simply do not start until it fits again.

A job that exceeds `--memory` is killed rather than slowed, and because the
runner dies mid-job GitHub can take several minutes to notice — the run stalls
with no further output and ends as cancelled. If a job stops producing output
part-way through and nothing on your side cancelled it, raise `--memory` first.

`fpcloud runner show <name>` says so directly while it is happening:

```
Problem   OOMKilled — exited with code 137
Note      a runner was killed for exceeding its memory limit; raise the pool's
          memory, and expect the job that was running to end as cancelled once
          GitHub gives up on it
```

The pool itself still reports `running` — the platform replaced the killed pod
straight away, so what failed is the job that was on it, not the pool.

## The image your job runs on

A runner pod runs the **bare GitHub agent image** — upstream's
`ghcr.io/actions/actions-runner`, tracking its releases. It is *not* GitHub's
`ubuntu-latest`, which preinstalls hundreds of tools: a step that shells out to
something the agent image does not carry (`xz`, `zstd`, most build tools) fails
with a plain `command not found`, and this is why.

Anything your workflow needs beyond the agent, bake into an image of your own
and point the pool at it:

```bash
fpcloud runner create ci --image ghcr.io/acme/runner:1
```

Two things that image must satisfy:

- **Start `FROM ghcr.io/actions/actions-runner`** (or this platform's image).
  The platform starts the container with the agent's own `/home/runner/run.sh`;
  an image without it never comes up.
- **Anonymously pullable.** Runner pods carry no registry credential, so a
  private image fails as `ImagePullBackOff`, not as a permission message.

## Building container images

There is no Docker daemon in a runner, and Docker-in-Docker is not available:
it needs a privileged container, which the platform does not run for anyone.
Instead, ask for a builder:

```bash
fpcloud runner create ci --builder ...
```

`--builder` puts a rootless [BuildKit](https://github.com/moby/buildkit)
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

The builder is a second container in the same pod, with its own size and its own
cost:

```bash
fpcloud runner create ci --builder-cpu 2 --builder-memory 4Gi ...
```

`--builder-cpu`/`--builder-memory` imply `--builder`, and either one on its own
is enough. Left unset, the builder takes the platform's default rather than a
copy of the runner's — the two containers do different work, and the size your
workflow needs says nothing about the size your `Dockerfile` needs. What the pool
costs is the runner's size plus the builder's; `fpcloud runner show` prints both,
whether or not you named them.

`fpcloud runner update <name> --no-builder` removes the builder again.

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
still name its label in `runs-on` will queue with nothing to pick them up, so
change them first.

## Limits

- Runners serve **your own** repositories. Scheduling workflows from forked pull
  requests is off — it would let anyone who opens a PR run code in your project.
- A pool is isolated at the same level as everything else in your project: a
  Kubernetes namespace. That is the right boundary for your own CI. It is not a
  sandbox for untrusted code, and nothing here claims to be one.
