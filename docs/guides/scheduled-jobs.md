
---
page_title: "Scheduled Jobs"
---

# Scheduled Jobs

A **job** is one schedule plus what to run. Each time it fires you get a **run**,
kept with its status, exit code and output — so a cron that quietly started
failing is visible instead of being noticed weeks later through missing side
effects. How much history is kept is [configurable](#how-long-runs-are-kept),
by count and by age.

Two kinds of target:

- **HTTP** — call an endpoint on a schedule. No image, no Dockerfile, nothing new
  to deploy: the platform sends the request. This is the common case ("POST
  `/internal/sweep` every 15 minutes").
- **Container** — run an image to completion. Use it when the work isn't
  reachable over HTTP.

## Schedule an HTTP call to your own app

```sh
fpcloud job create sweep \
  --app api \
  --schedule "*/15 * * * *" \
  --http-target /internal/sweep
```

`--app` is what makes this cheap. The job inherits that app's config and secrets,
and a path target resolves to the app's **in-cluster address** — the request never
leaves the cluster, never touches your public hostname, and needs no egress rules.

Authenticate it with a value your app already has:

```sh
fpcloud config set CRON_TOKEN=s3cr3t --app api
fpcloud job create sweep --app api --schedule "*/15 * * * *" \
  --http-target /internal/sweep \
  --header 'Authorization: Bearer $CRON_TOKEN'
```

**Single quotes.** `$CRON_TOKEN` has to reach the platform literally: it is
resolved at fire time from the job's environment, which is the app's config, not
from your shell. Double quotes would expand it locally to an empty string and
store the header as `Bearer `, and the self-call would 401 on every fire. The
token stays in one place.

The job reaches `/internal/sweep` from inside the cluster, but on a public app
(`--ingress all`) that path is also served on your public hostname. Keep it off
the edge:

```sh
fpcloud app set-routes api --route /internal/
```

The job keeps working — it never goes through the ingress — while an outside
request to `/internal/sweep` is refused before it reaches your app. See
[Route visibility](cli-and-terraform.md#route-visibility-keeping-a-path-off-the-public-ingress).

An absolute URL works too, for calling something outside your app:

```sh
fpcloud job create ping --schedule "0 * * * *" \
  --http-target https://example.com/hook --method POST
```

Reaching the public internet requires the project's egress to allow it
(`fpcloud project update --egress https`).

A run succeeds when the endpoint answers `2xx`. Anything else fails the run and
retries per `--max-retries`.

## Schedule a container

```sh
# the app's own image, different entrypoint
fpcloud job create nightly --app api --schedule "0 3 * * *" \
  --command "npm run cleanup"

# an unrelated image
fpcloud job create report --schedule "0 6 * * 1" \
  --image ghcr.io/acme/weekly-report:v2 --arg --format=pdf
```

With `--app`, the job follows the app: deploy the app and its jobs pick up the
new image on the next fire. Without it, `--image` is what runs, exactly as
pinned.

## Timezone

Schedules are read in **UTC** unless you say otherwise:

```sh
fpcloud job create nightly --app api --schedule "0 3 * * *" --timezone Europe/Stockholm
```

Give a tz database name and "03:00 local" stays 03:00 across daylight-saving
changes, instead of drifting by an hour twice a year.

## Overlapping runs

If a run is still going when the next fire is due, `--concurrency` decides:

| Value | Behavior |
|---|---|
| `forbid` (default) | Skip the new run. A sweep that overruns never runs twice at once. |
| `replace` | Kill the running one and start fresh. |
| `allow` | Run them in parallel. |

## Assume some fires will be missed

A scheduled job is not a guarantee that every fire happens. A run can be skipped
because the previous one is still going (`--concurrency forbid`), because the
platform itself was unable to start it, or because it started and failed every
retry. If a fire is more than five minutes late, it is dropped rather than
started — a job that fires every 15 minutes and is unreachable for two hours
loses those eight fires; it does not run them all at once when service returns.

That five-minute window is a **boundary, not a tuning knob**: it is not
configurable, and there is no queue behind it. Design for "this fire may never
happen", not for "it will start eventually".

A dropped fire is also **invisible from both ends**. A run that never started
leaves no run record, so nothing in `job runs` or the console marks the gap — the
history shows the fires that happened, not the ones that should have. And a job
that reconciles from current state looks identical whether it ran on time or not.
So the first time anyone notices is when a delta-based job has quietly lost work,
which is already too late to recover it.

That makes the design of the work the thing that matters:

- **Derive the work from current state** and a missed fire costs latency only.
  "Send to everyone unnotified", "retry everything still marked failed", "delete
  everything past its expiry" all pick up whatever accumulated while the job was
  not running, so the next successful fire is a full recovery.
- **Derive it from "what changed since I last ran"** and a missed fire loses that
  work permanently. A job that reads a cursor, processes the delta, and advances
  the cursor only when it succeeds is fine; one that assumes it runs on every
  tick is not.

Write the first kind whenever you can. When you can't, make the job record its
own progress so a later run can tell what was skipped — the platform's run
history tells you a fire did not happen, not what it would have done.

## Retries and timeouts

```sh
fpcloud job create sweep --app api --schedule "*/15 * * * *" \
  --http-target /internal/sweep \
  --max-retries 3 --timeout 120
```

`--max-retries` (default 3) is how many times a failed run is retried; the delay
between attempts is an exponential backoff starting at ten seconds and capping at
six minutes. `--timeout` (default 300 seconds, max 12 hours) kills a run that
overruns — the attempt counts as failed.

## Run it now

Verifying a schedule shouldn't mean waiting for the next window:

```sh
fpcloud job run sweep            # fire it, return immediately
fpcloud job run sweep --wait     # fire it and print the output when it finishes
```

A manual run ignores the concurrency policy — you asked for it, so it runs.

## See what happened

```sh
fpcloud job list                 # every job, with its last run
fpcloud job runs sweep           # this job's history
fpcloud job logs sweep           # output of the most recent run
fpcloud job logs sweep --run sweep-manual-1753500000
```

`fpcloud job list` shows the last run's status inline, so a failing schedule is
visible from the top-level list.

A run's **output** comes from the platform's log store, which keeps 14 days — so
a run whose outcome you can still see may be older than the lines it printed.
Its status, exit code and timing are kept for as long as the run is.

## How long runs are kept

History is bounded two ways at once, and a run is dropped when either bound is
exceeded:

```sh
fpcloud job update sweep --keep-runs 20            # newest N, both outcomes
fpcloud job update sweep --retain-succeeded 24h    # completed runs age out fast
fpcloud job update sweep --retain-failed 30d       # errored runs linger
```

The two windows are separate because a failure stays worth reading long after a
success is noise — the run you want weeks later is the one that broke. Accepted
units are `h`, `m`, `s`, plus `d` and `w`; `never` turns the age limit off and
leaves `--keep-runs` as the only bound.

Defaults are `--keep-runs 10`, `--retain-succeeded 7d`, `--retain-failed 30d`.

This governs both the stored run history and how long the run's pod lingers in
your project, so a job that fires rarely no longer leaves finished pods sitting
around for months. Note that a *completed* run may outlive its window slightly:
its pod is created before the outcome exists, so it starts on the longer of the
two windows and is narrowed once the run has finished.

## Pause a schedule

```sh
fpcloud job update sweep --suspended        # stop firing
fpcloud job update sweep --suspended=false  # resume
```

A suspended job keeps its history and can still be triggered with `job run`.

## Change or remove a job

```sh
fpcloud job update sweep --schedule "*/30 * * * *"
fpcloud job show sweep
fpcloud job delete sweep
```

The name is fixed at creation; everything else can be changed in place. Deleting
a job removes its schedule and its run history.

## In the console

Everything above is on the **Jobs** page of the web console: create a job, run
one on demand, pause or resume a schedule, and open a job's run history to read
the captured output of any run. Retention is edited from the run history itself,
next to the list it governs.

## Terraform

```hcl
resource "fpcloud_job" "sweep" {
  project     = fpcloud_project.demo.name
  name        = "sweep"
  app         = fpcloud_app.api.name
  schedule    = "*/15 * * * *"
  timezone    = "Europe/Stockholm"
  http_url    = "/internal/sweep"
  http_method = "POST"
  http_headers = {
    Authorization = "Bearer $CRON_TOKEN"
  }
  concurrency = "forbid"
  max_retries = 3

  keep_runs                = 10
  retain_succeeded_seconds = 86400   # 1d
  retain_failed_seconds    = 2592000 # 30d
}
```

Keep the schedule next to the app it belongs to; `tofu apply` is then the one
place both are defined.

## Notes

- One schedule per job. Two schedules means two jobs — which keeps "when did this
  last run" unambiguous.
- A run's output is captured when it finishes and kept with the run record, so
  history survives longer than the underlying pod.
- `--keep-runs` (default 10) bounds history by count; `--retain-succeeded` and
  `--retain-failed` bound it by age, per outcome.
- Runs count against the project's pod quota while they execute; a job firing
  every minute is possible but wasteful.
