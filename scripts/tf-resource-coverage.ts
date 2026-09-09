#!/usr/bin/env -S deno run --allow-read --allow-run --allow-env
// tf-resource-coverage — does every mutating client method have an actual
// Terraform resource/data-source calling it, not just client-layer plumbing?
//
// pkg/client having a method only proves the CLI can reach an endpoint — the
// client can carry a method for years with zero Terraform surface on top of it
// (registry retention, org secrets, FKE). This reads every func on *Client that
// wraps a newRequest() call out of the github.com/fogpipe/cloud-cli module, and
// cross-references it against every `.client.Method(` call in this repo's
// internal/provider/*.go.
//
// The client is read at the version go.mod pins, so the verdict is a fact
// about this commit: a gap is red on the bump that brings the method in, and
// the same tree reads the same everywhere (scripts/client-module.ts says why
// "latest release" was not that). Whether the pin itself lags the released
// client is asked at release time, not here.
//
// New gaps are failures; existing ones are accepted via a checked-in
// baseline (scripts/tf-resource-coverage-baseline.txt) so this doesn't try to
// retroactively judge every pre-existing intentional CLI-only endpoint
// (auth, imperative actions like rollback/deploy, data-plane object ops).
// Shrink the baseline as real coverage gets built; grow it only for a
// deliberate, reviewed new exclusion, by editing the file and writing the
// reason beside the entry. This script reads that file and never writes it: it
// used to, behind --update-baseline, which regenerated it from the gap list and
// deleted every reason and grouping in it (fogpipe/cloud-workspace#888).
//
// Usage:
//   deno run --allow-read --allow-run --allow-env scripts/tf-resource-coverage.ts

import { clientModule, pinnedClientDir } from "./client-module.ts";

// The baseline is not written from here (fogpipe/cloud-workspace#888). Refuse
// the flag that used to do it rather than ignoring it: an unknown argument runs
// the ordinary gate and reports green, which reads exactly like a refresh that
// worked and leaves the reasons in the file the caller meant to update.
if (Deno.args.includes("--update-baseline")) {
  console.error(
    "--update-baseline is gone: it regenerated\n" +
      "scripts/tf-resource-coverage-baseline.txt from the gap list and deleted\n" +
      "every reason and grouping recorded in it " +
      "(fogpipe/cloud-workspace#888).\n" +
      "Run without it and edit that file by hand — the entries to add and the\n" +
      "ones that have gone stale are both printed.",
  );
  Deno.exit(2);
}

const root = new URL("../", import.meta.url).pathname;
const baselinePath = root + "scripts/tf-resource-coverage-baseline.txt";

// --- client.go: every *Client method that actually wraps an API call ---
function clientMethods(clientGoPath: string): Set<string> {
  const lines = Deno.readTextFileSync(clientGoPath).split("\n");
  const methods = new Set<string>();
  let current: string | null = null;
  let sawRequest = false;
  const flush = () => {
    if (current && sawRequest) methods.add(current);
    current = null;
    sawRequest = false;
  };
  for (const line of lines) {
    const fn = line.match(/^func \(c \*Client\) ([A-Za-z0-9_]+)\(/);
    if (fn) {
      flush();
      current = fn[1];
      continue;
    }
    if (current && /\bnewRequest\(/.test(line)) sawRequest = true;
    if (current && /^}$/.test(line)) flush();
  }
  flush();
  return methods;
}

// --- internal/provider/*.go: every `.client.Method(` call site ---
function calledMethods(providerDir: string): Set<string> {
  const called = new Set<string>();
  for (const entry of Deno.readDirSync(providerDir)) {
    if (!entry.isFile || !entry.name.endsWith(".go")) continue;
    if (entry.name.endsWith("_test.go")) continue;
    const src = Deno.readTextFileSync(`${providerDir}/${entry.name}`);
    for (const m of src.matchAll(/\.client\.([A-Za-z0-9_]+)\(/g)) {
      called.add(m[1]);
    }
  }
  return called;
}

const client = pinnedClientDir();
const available = clientMethods(`${client.dir}/pkg/client/client.go`);
const called = calledMethods(`${root}internal/provider`);

const uncovered = [...available].filter((m) => !called.has(m)).sort();

const baseline = new Set(
  (() => {
    try {
      return Deno.readTextFileSync(baselinePath).split("\n").map((l) =>
        l.trim()
      ).filter((l) => l && !l.startsWith("#"));
    } catch {
      return [];
    }
  })(),
);

const newGaps = uncovered.filter((m) => !baseline.has(m));
const stale = [...baseline].filter((m) => !uncovered.includes(m)).sort();

console.log(
  `${clientModule} ${client.version}: ${available.size} client methods, ${uncovered.length} uncovered by any TF resource, ${baseline.size} in baseline.\n`,
);

if (newGaps.length) {
  console.log("NEW gaps — a client method with no Terraform resource surface:");
  for (const m of newGaps) console.log(`  ✗ ${m}`);
  console.log(
    "\nEither wire it into a resource/data-source, or if it's intentionally " +
      "CLI/API-only, paste the line(s) above into " +
      "scripts/tf-resource-coverage-baseline.txt,\nin the group it belongs to, " +
      "with the reason beside it.",
  );
} else {
  console.log("no new gaps ✓");
}

if (stale.length) {
  console.log(
    `\n${stale.length} baseline entries are now covered — remove from the baseline:`,
  );
  for (const m of stale) console.log(`  · ${m}`);
}

Deno.exit(newGaps.length ? 1 : 0);
