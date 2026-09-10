
---
page_title: "GitHub Actions Runners"
---

# GitHub Actions Runners

A **runner** is not a machine. Nothing runs until GitHub has a job for it: the
platform starts a pod, that pod serves exactly one job, and it is destroyed
when the job ends. Between jobs there is nothing, so an idle runner costs
nothing, and no state carries from one job to the next.

A project has **one** runner. You pick its size from a menu and say how many
jobs it may run at once; there is nothing to name.

Runners live in your project's namespace, beside your apps and databases, and
are isolated from other tenants exactly the way those are.

## Connect your GitHub account

Once per project, connect the GitHub account your runner will serve:

```bash
fpcloud github connect
```

This opens GitHub and asks you to authorize as yourself, then waits and prints
the outcome — connected, or refused and why. Fogpipe records the account
against this project. That is the entire setup: nothing to copy, no key to
handle, and no organization name to type. `--no-wait` prints the link and
returns at once; the outcome is recorded either way, and `fpcloud github
status` reads it back, including the reason for a refusal and the install link
it carries.

Only accounts **you administer** can be connected — an owner of the
organization, or your own user account. Being a member is not enough, because
connecting lets a project run CI on that account. Fogpipe never takes an
organization name on trust, so no project can point a runner at an account it
does not control.

If the **Fogpipe** app is not installed on the account yet, connecting tells you
and gives you the link. If you administer more than one account with it
installed, say which — a named account is checked before the browser opens, so
a missing installation is reported there and then:

```bash
fpcloud github connect --account acme
```

```bash
fpcloud github status      # which account this project is connected to
fpcloud github disconnect  # remove the binding (delete the runner first)
```

## Create the runner

The runner needs nothing from you — it serves every repository in the connected
account:

```bash
fpcloud runner create
```

Then use it from a workflow. Its `runs-on` label is your project name followed
by `-ci` — so the runner in project `acme` is `acme-ci`:

```yaml
jobs:
  test:
    runs-on: acme-ci
    steps:
      - uses: actions/checkout@v5
      - run: make test
```

The label carries the project because GitHub registers runners per
**account**, and several of your projects can share one account. Without the
project in the name, two projects would claim the same label — and jobs would
land in whichever project's runner GitHub happened to pick.

`fpcloud runner create` prints the exact label, and `fpcloud runner show`
repeats it, so you never have to assemble it yourself.

## Check a workflow before you push it

```bash
fpcloud runner check
fpcloud runner check --workflow .github/workflows/ci.yml
```

Reads `.github/workflows` (or the file you name) and says which of its jobs
your runner can actually take. It sends the file's text, so it answers for a
workflow you have not pushed yet — which is the point: a job that names a label
no runner serves does not fail, it **queues forever**, and nothing on GitHub
says why.

Each job gets one verdict:

| Verdict | What it means |
|---|---|
| `runs` | the label is this project's runner, and the job asks for nothing it cannot serve |
| `queues` | no runner here serves that label — the job waits with nothing to pick it up |
| `refused` | the label matches, but the job declares `container:` or `services:`. A `services:` block moves to the runner — see **Service containers** below |
| `github` | a GitHub-hosted label (`ubuntu-latest` and friends): it runs, on GitHub's minutes rather than on your runner |
| `undetermined` | `runs-on` is an expression, or the job calls a reusable workflow, so which runner it picks is not in this file |

`undetermined` is never "fine" — it is the check saying it could not answer.
A matrix over `runs-on: ${{ matrix.os }}` has to be read by you.

The commonest finding is `runs-on: self-hosted`, which works on most
self-hosted setups and never here: your runner registers under exactly one
label, its own, so `self-hosted` matches nothing. Use the label `runner create`
printed.

Nothing reads your repository — the platform holds no copy of your workflows
and never fetches one.

`runner show` also says what the runner is doing: **Busy** is pods executing a
job beside the most it may run at once (`3 of 4`), one line per busy pod names
the job and repository it serves, and **Waiting** is GitHub's own count of
jobs handed to the runner that no pod has started — read off the runner's
listener, never reconstructed. That last number is the whole diagnosis: a
runner with nothing waiting is idle, one with jobs waiting and every pod busy
is undersized, and one with jobs waiting and no pod busy cannot schedule (see
`--max` and the org ceiling below). A queue the platform could not read is
reported as unreadable, and `project status` shows it as `?` — never as an
empty queue.

The same per-account registration is the runner's reach: a workflow in a
repository **outside** the connected account never sees it. Naming the label
there is not an error on either side — the job queues, waiting for a runner
GitHub will never offer it. Both commands say which account the label works
in, and `fpcloud webhook setup` warns when the repository it is given is
outside it.

## Bringing your own credential

The Fogpipe app is the default and needs nothing from you. Two cases it cannot
serve, both selected explicitly:

These need `--github-account`, because a key or a token says nothing about which
account it is for. Holding the credential is itself the proof it is yours:

```bash
# your own GitHub App — for an organization whose policy forbids third-party apps
fpcloud runner create --credential app --github-account acme \
    --github-app-id 123456 \
    --github-app-installation-id 7891011 \
    --github-app-private-key-file ./acme-ci.private-key.pem

# a personal access token — fine for a first try
fpcloud runner create --credential token --github-account acme --github-token ghp_…
```

Your own app needs the **Self-hosted runners: Read & write** organization
permission. A token carries a person's full access and dies with their account,
so it is not something to leave in place.

Anything secret you supply is encrypted on arrival and **write-only**: it is
never returned by the API, the CLI or the console. Rotate it by supplying a new
one:

```bash
fpcloud runner update --credential app --github-app-private-key-file ./new-key.pem
```

A credential is replaced whole. A runner authenticates as an app *or* as a
token, never as half of each.

## Sizing

```bash
fpcloud runner create --size large --max 4
```

`--size` is the container your workflow's steps execute in. Three sizes, and
they are the only ones:

| Size | CPU | Memory |
|---|---|---|
| `small` | 1 | 2Gi |
| `medium` (default) | 2 | 4Gi |
| `large` | 4 | 8Gi |

A menu rather than two numbers because the number that matters is how many
jobs the platform can run beside each other, and three shapes are what it can
plan for. A builder, if you ask for one, is sized separately and adds to what
each job costs.

The two columns bound differently. **Memory is a ceiling**: a job that exceeds
it is killed. **CPU is a share**: the size is what your job is scheduled and
billed as, and what it is guaranteed against other jobs when the node is busy —
but a job on a quiet node runs on whatever cores are idle, above its size, at
no extra cost. The same goes for disk: a build has the whole device to itself
until something else on the node needs it, and then yields most of it. A job
that runs faster than its size suggests is not being over-billed; a job that
runs slower is sharing a busy node, and `fpcloud runner show` says nothing
about that because nothing has failed.

`--max` is how many jobs the runner runs at once — one pod each, `2` by
default, up to `8`. Jobs beyond it queue on GitHub. Every one of them costs
cores and memory for as long as it runs, so this is a budget, not a throughput
dial. There is no floor: the runner always scales to zero, and a job waits a
few seconds for its pod.

```bash
fpcloud runner update --size medium --max 2
```

### Runners draw on your organization's resource ceiling

Your runner's pods run in a namespace of their own, bounded by the same
ceiling as the rest of your organization — **8 CPU / 16Gi / 20 pods** by default
(see [the organization's resource
ceiling](projects-and-access.md#the-organizations-resource-ceiling)). CI
spends the same budget your apps and databases do, so a runner you declare is
capacity they no longer have.

The rule is that **one job must fit**: one pod — the size you chose, plus its
builder, plus every service container — has to fit inside what the ceiling has
left. `--max` is weighed differently: the runner's namespace is bounded at
`--max` pods of that shape, so a runner may ask for more concurrency than the
ceiling holds at once, and those jobs wait for a slot. `fpcloud runner show`
says how many of `--max` the ceiling admits right now.

A runner too large to run even once is refused when you ask for it, rather
than accepted and left with jobs that never start:

```console
$ fpcloud runner create --size large --builder-memory 8Gi
Error: this needs 16Gi memory, and organization acme has 4Gi of its 4Gi memory
ceiling left; shrink it, free capacity in another project, or ask your operator
to raise the ceiling
```

An existing runner that stops fitting — because its builder grew, or the
ceiling was lowered — keeps its declaration and says the same thing on
`fpcloud runner show`. It is the runner you edit to fix it, so it is not taken
away from you; its pods simply do not start until it fits again.

The size and the builder are weighed together, so shrink them in one command
when both have to give:

```console
$ fpcloud runner update --size small --builder-memory 2Gi
```

A job that exceeds its size's memory is killed rather than slowed, and because
the pod dies mid-job GitHub can take several minutes to notice — the run
stalls with no further output and ends as cancelled. If a job stops producing
output part-way through and nothing on your side cancelled it, go up a size
first.

`fpcloud runner show` says so directly while it is happening:

```
Problem   OOMKilled — exited with code 137
Note      a runner was killed for exceeding its memory limit; pick a larger
          size, and expect the job that was running to end as cancelled once
          GitHub gives up on it
```

The runner itself still reports `running` — the platform replaced the killed
pod straight away, so what failed is the job that was on it, not the runner.

## The image your job runs on

A runner pod runs the **bare GitHub agent image plus a small, deliberate set of
additions** — it is *not* GitHub's `ubuntu-latest`, which preinstalls hundreds
of tools. A step that shells out to something missing fails with a plain
`command not found`, and this is why.

The image is the platform's, built from upstream's
`ghcr.io/actions/actions-runner` and tracking its releases. On top of the
agent it adds:

- `xz-utils` — `tar -J`, `.xz` artifacts and nix cache actions all pipe
  through `xz`
- `buildx` and a `docker` front end over the builder, so an existing build
  script works unchanged (see **Building container images**)

Anything else your workflow needs, install in a step — the way you would on
GitHub's own runners for a tool `ubuntu-latest` lacks — or run it in a
[service container](#service-containers) beside the job.

## Building container images

There is no Docker daemon in a runner, and Docker-in-Docker is not available:
it needs a privileged container, which the platform does not run for anyone.
Instead, ask for a builder:

```bash
fpcloud runner create --builder
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

### Your existing build scripts work as they are

If your build already lives in a shell script that calls `docker`, it does not
need rewriting. A runner with a builder carries a `docker` command that speaks
to the builder instead of to a daemon:

```bash
docker build --platform linux/amd64 -t registry.cloud.fogpipe.com/acme/api:v1 .
docker push registry.cloud.fogpipe.com/acme/api:v1
```

Both work unchanged. There is no local image store to push *from*, so `push`
completes the build you already asked for, publishing it — that is a cache hit
against the builder rather than a second build, and the image is the one your
`build` described. `docker build --push` in one step does the same thing more
directly, and `docker tag` names a build again before pushing it.

Anything that genuinely needs a daemon — `docker run`, `ps`, `images`, `pull`,
`compose` — is **refused with a message saying why**, not quietly ignored. A
job that needs to run the image it just built should run the build's test stage
inside the `Dockerfile`, or pull the pushed image in a later job.

`docker buildx …` works too, and `buildctl` is there if you would rather use
BuildKit's own client.

### A cache that outlives the job

The pod is per-job, so nothing on its filesystem survives — but a cache does not
have to live on the filesystem. BuildKit can keep its layer cache in a registry,
and you already have one: the cache is an ordinary repository in your project,
alongside your images.

```bash
CACHE="$(fpcloud registry repo-path buildcache)"

docker buildx build \
  --cache-from "type=registry,ref=$CACHE" \
  --cache-to   "type=registry,ref=$CACHE,mode=max,ignore-error=true" \
  --push -t "$(fpcloud registry repo-path api):$GITHUB_SHA" .
```

`fpcloud registry repo-path` prints the path your project pushes to, so nothing
has to be typed twice or kept in step with a rename. In a workflow, the
`fogpipe/cloud-actions/build-cache` action derives the same reference and hands
it back as an output.

Three things follow from it being an ordinary repository, and none of them is a
special case:

- It is **yours**, on your project's own path, and no other project can read or
  write it.
- It **counts against your organization's registry storage**, like any image,
  and a push is refused when there is no room left. `fpcloud registry repos
  list` shows what it holds.
- You can have **several** — one per image you build — because they are separate
  repositories. Deleting one touches nothing else.

`ignore-error=true` on the export is deliberate: a cache is an optimisation, and
a registry that will not take it should cost you a cold build rather than a
failed one.

Without a cache, every build starts cold, because the pod is per-job.

The builder is a second container in the same pod, with its own size and its own
cost:

```bash
fpcloud runner create --builder-cpu 2 --builder-memory 4Gi
```

`--builder-cpu`/`--builder-memory` imply `--builder`, and either one on its own
is enough. Left unset, the builder takes the platform's default rather than a
copy of the runner's size — the two containers do different work, and the size
your workflow needs says nothing about the size your `Dockerfile` needs. What a
job costs is the runner's size plus the builder's; `fpcloud runner show` prints
both, whether or not you named them.

`fpcloud runner update --no-builder` removes the builder again.

## Pushing to your Fogpipe registry

A runner is an ordinary workload in your project, so it authenticates to the
Fogpipe registry the same way your CI does anywhere else: through OIDC
federation, with no stored credential. See
[Deploy from GitHub](deploy-from-github.md).

## Watching the runner

```bash
fpcloud runner show
```

`Status` is the runner's own state, not a job's. `pending` means the platform
has declared the runner and is waiting for it to register with GitHub — normal
for a few seconds after creation, and worth investigating if it persists
(usually a credential without the self-hosted-runner permission).

`Busy` is how many pods exist right now, which is how many of your jobs are
running.

## Where runners run

Runner pods get a namespace of their own, alongside your project's rather than
inside it. It is per-project — no other tenant's jobs share it — and it exists
so the credential the runner registers with is somewhere nothing you run can
read, including the code in your own workflows.

Two things follow from that. Runner pods are allowed outbound HTTPS, so they
reach GitHub, actions and toolchains without you opening your project's egress.
And they **cannot** reach your project's own services by their in-cluster names:
a job that needs your database or your app should go through its public
address, or use a **service container** (below).

## Service containers

A workflow that needs a database beside it declares it on the **runner**, not
in the workflow:

```bash
fpcloud runner create \
  --service postgres=postgres:18-alpine \
  --service-env postgres=POSTGRES_PASSWORD=hunter2 \
  --service-env postgres=POSTGRES_DB=app \
  --service-memory postgres=1Gi
```

Every job then gets that container beside it, and reaches it on **`127.0.0.1`**
on whatever port the image listens on — there is no port to publish, because
there is no network boundary between the containers of a pod:

```yaml
jobs:
  test:
    runs-on: acme-ci
    steps:
      - uses: actions/checkout@v5
      - run: psql postgres://postgres:hunter2@127.0.0.1/app -c 'select 1'
```

A runner can carry up to five, each with its own `--service-cpu` and
`--service-memory`. Unset, a service gets its own default (500m / 1Gi) rather
than a copy of the runner's size — its appetite has nothing to do with how big
the job is. All of them count towards your organization's ceiling for as long as
a job is running, because all of them are in the pod.

A service image is pulled by the pod, which carries no ambient registry
credential. A **private** image is supported on exactly one host: the platform
registry, under your own project's path
(`registry.cloud.fogpipe.com/<org>/<project>/...`) — push it there and the pod
pulls it with a platform-managed credential scoped to that path. A path outside
your project is refused when the runner is created. On any other registry the
image must be anonymously pullable, or it fails as `ImagePullBackOff`.

`fpcloud runner update --service …` replaces the whole set, and
`--no-services` removes them.

### Why not `services:` in the workflow

GitHub serves a job's own `services:` block by running the job **inside a
container**, which needs a runner container mode this platform does not run —
the same reason there is no Docker daemon here. So a `services:` block does not
work and will not; `fpcloud runner check` reports it as `refused` and points
here.

The runner is also the only place the two can coexist. A container mode would
run your steps in a separate pod, which puts the image builder out of reach — so
a runner could have service containers or `docker build`, never both.

**A service's environment is not a secret store.** It is stored and shown as
written, and it configures a container that lives for one job and is reachable
from nothing but that job's own pod. That is the same thing GitHub does with
`services.*.env`, which sits in plaintext in your repository. A credential to
anything that outlives the job does not belong there — put it in the workflow's
own secrets and pass it to the step.

## Restarting the runner

```bash
fpcloud runner restart            # drain: running jobs finish, then every pod is new
fpcloud runner restart --force    # now: running jobs are killed
fpcloud runner restart --no-wait  # accepted; `runner show` reports the rest
```

A runner that has stopped taking work — a listener wedged on GitHub, a pod
that will never finish — is recycled with `restart`, not by deleting anything.
The platform accepts the request and carries it out: the runner's pods are
replaced and its listener with them. **Draining is the default**: a pod in the
middle of a job finishes it first, and because a pod serves exactly one job
the drain ends with the longest running job rather than never. While it waits,
the command and `runner show` say which jobs it is waiting on, so a restart
held by a twelve-minute job is not mistaken for a hang.

`--force` is for the runner that a drain cannot recover — one wedged on a pod
that will never finish. It replaces every pod now and kills whatever they are
running; GitHub does not re-offer a job whose runner died, so the jobs are
lost, not retried. It asks before it does that. The safe action is the one you
get without typing anything.

## Removing the runner

```bash
fpcloud runner delete
```

The runner is deregistered from GitHub and its pods go with it. Workflows that
still name its label in `runs-on` will queue with nothing to pick them up, so
change them first.

## Limits

- Runners serve **your own** repositories. Scheduling workflows from forked pull
  requests is off — it would let anyone who opens a PR run code in your project.
- A runner is isolated at the same level as everything else in your project: a
  Kubernetes namespace. That is the right boundary for your own CI. It is not a
  sandbox for untrusted code, and nothing here claims to be one.
