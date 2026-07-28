
---
page_title: "Scheduled Jobs"
---

# Scheduled Jobs

A **job** is one schedule plus what to run. Each time it fires you get a **run**,
and the last ten runs are kept with their status, exit code and output — so a
cron that quietly started failing is visible instead of being noticed weeks later
through missing side effects.

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
  --header "Authorization: Bearer $CRON_TOKEN"
```

Quote it so your shell doesn't expand it: `$CRON_TOKEN` is resolved at fire time
from the job's environment, which is the app's config. The token stays in one
place.

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
the captured output of any run.

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
}
```

Keep the schedule next to the app it belongs to; `tofu apply` is then the one
place both are defined.

## Notes

- One schedule per job. Two schedules means two jobs — which keeps "when did this
  last run" unambiguous.
- A run's output is captured when it finishes and kept with the run record, so
  history survives longer than the underlying pod.
- `--keep-runs` (default 10) bounds how much history is retained per job.
- Runs count against the project's pod quota while they execute; a job firing
  every minute is possible but wasteful.
