#!/usr/bin/env -S deno run --allow-read --allow-run --allow-env
// tf-field-coverage — can a Terraform user reach every field of the resource?
//
// tf-resource-coverage.ts asks whether a client METHOD has a resource calling
// it, and stops there: once `fpcloud_runner` calls CreateRunner, that gate is
// satisfied forever, no matter what the runner grows afterwards. So a field
// could be added to pkg/client, shipped in the CLI, documented, and remain
// unsettable in HCL with every gate green.
//
// That is not hypothetical: `builds` became `builder` in cloud-platform#850, and
// the only thing that would have caught a provider left behind was someone
// remembering.
//
// It pairs client type `X` with this repo's `XResourceModel` — name-only
// pairing, deliberately: pairing by shape would invent relationships nobody
// declared.
//
// Direction is deliberate: a field the client carries and Terraform cannot
// express is a failure. The reverse cannot happen silently — the provider reads
// client fields in Go, so one that disappears fails the build.
//
// The client is resolved at its LATEST RELEASE, not at the version go.mod pins
// here; see tf-resource-coverage.ts for why the pin is the wrong yardstick.
//
// Nested objects are compared one level down, against the attribute names
// declared inside that attribute's own schema block, because that is where the
// last drift was.
//
// New gaps are failures; reviewed permanent ones live in
// scripts/tf-field-coverage-baseline.txt, the same shape as its sibling.
//
// Usage:
//   deno run --allow-read --allow-run --allow-env scripts/tf-field-coverage.ts
//   deno run --allow-write --allow-read --allow-run --allow-env scripts/tf-field-coverage.ts --update-baseline

const root = new URL("../", import.meta.url).pathname;
const baselinePath = root + "scripts/tf-field-coverage-baseline.txt";

// `go list -m` answers with an empty Dir — not an error — when the module is
// known but not yet downloaded, the normal state of a fresh CI checkout.
// (Same helper as tf-resource-coverage.ts, same reason.)
function moduleDir(mod: string): string {
  const go = (...args: string[]) => {
    const out = new Deno.Command("go", {
      args,
      cwd: root,
      env: { ...Deno.env.toObject(), GOWORK: "off" },
    }).outputSync();
    if (!out.success) {
      throw new Error(`go ${args.join(" ")} failed: ${new TextDecoder().decode(out.stderr)}`);
    }
    return new TextDecoder().decode(out.stdout).trim();
  };
  let dir = go("list", "-m", "-f", "{{.Dir}}", mod);
  if (dir === "") {
    go("mod", "download", mod);
    dir = go("list", "-m", "-f", "{{.Dir}}", mod);
  }
  if (dir === "") throw new Error(`${mod} is not in the module cache after go mod download`);
  return dir;
}

type Field = { json: string; type: string };

// Struct name → its marshalled fields, each keeping the Go type so a nested
// object can be followed. A field tagged `json:"-"` is not on the wire and an
// untagged one cannot be matched by name across two hand-written declarations,
// so both are skipped rather than guessed at.
function goStructs(src: string): Map<string, Field[]> {
  const out = new Map<string, Field[]>();
  for (const m of src.matchAll(/^type (\w+) struct \{\n([\s\S]*?)^\}/gm)) {
    const fields: Field[] = [];
    for (const line of m[2].split("\n")) {
      const decl = line.match(/^\s*\w+\s+([\w\.\[\]\*]+)\s+`[^`]*json:"([^",]+)/);
      if (decl && decl[2] !== "-") fields.push({ json: decl[2], type: decl[1] });
    }
    out.set(m[1], fields);
  }
  return out;
}

// Provider model name → the tfsdk attribute names it binds, plus the file it
// was declared in (the schema to read for nested attributes is that file's).
function providerModels(dir: string): Map<string, { attrs: Set<string>; src: string }> {
  const out = new Map<string, { attrs: Set<string>; src: string }>();
  for (const entry of Deno.readDirSync(dir)) {
    if (!entry.isFile || !entry.name.endsWith(".go")) continue;
    if (entry.name.endsWith("_test.go")) continue;
    const src = Deno.readTextFileSync(`${dir}/${entry.name}`);
    for (const m of src.matchAll(/^type (\w+) struct \{\n([\s\S]*?)^\}/gm)) {
      const attrs = new Set<string>();
      for (const line of m[2].split("\n")) {
        const tag = line.match(/`[^`]*tfsdk:"([^"]+)"/);
        if (tag) attrs.add(tag[1]);
      }
      if (attrs.size) out.set(m[1], { attrs, src });
    }
  }
  return out;
}

// Every attribute name the resource's own file mentions, however it is built —
// a tfsdk tag on a nested model, a key in a schema literal, or a key whose value
// comes from a helper (`"liveness": probeSchema(…)`).
//
// Scoped to the file rather than to the nested attribute's own block, and so
// blind to a child declared at the wrong level. That is the deliberate trade:
// the provider has three different ways of declaring a nested object and a check
// strict about which one was used reports drift that is only style. What it does
// catch is the thing that actually happens — the client grows a field the
// provider has never heard of.
function attrNames(src: string): Set<string> {
  return new Set([
    ...[...src.matchAll(/`[^`]*tfsdk:"([^"]+)"/g)].map((m) => m[1]),
    ...[...src.matchAll(/"(\w+)":\s*\w*[Ss]chema\./g)].map((m) => m[1]),
    ...[...src.matchAll(/"(\w+)":\s*\w+\(/g)].map((m) => m[1]),
  ]);
}

const clientTypes = goStructs(
  Deno.readTextFileSync(
    `${moduleDir("github.com/fogpipe/cloud-cli@latest")}/pkg/client/types.go`,
  ),
);
const models = providerModels(`${root}internal/provider`);

const gaps: string[] = [];
const paired: string[] = [];
for (const [name, fields] of [...clientTypes].sort()) {
  const model = models.get(`${name}ResourceModel`);
  if (!model) continue;
  paired.push(name);
  for (const field of fields) {
    if (!model.attrs.has(field.json)) {
      gaps.push(`${name}.${field.json}`);
      continue;
    }
    // A field carrying a struct of its own is a nested attribute, and its
    // children are reachable only if the resource names them somewhere.
    const inner = clientTypes.get(field.type.replace(/^[\*\[\]]+/, ""));
    if (!inner) continue;
    const declared = attrNames(model.src);
    for (const sub of inner) {
      if (!declared.has(sub.json)) gaps.push(`${name}.${field.json}.${sub.json}`);
    }
  }
}
gaps.sort();

const baseline = new Set(
  (() => {
    try {
      return Deno.readTextFileSync(baselinePath).split("\n").map((l) => l.trim())
        .filter((l) => l && !l.startsWith("#"));
    } catch {
      return [];
    }
  })(),
);

if (Deno.args.includes("--update-baseline")) {
  Deno.writeTextFileSync(
    baselinePath,
    "# client fields with no Terraform attribute expressing them, reviewed and\n" +
      "# accepted. generated by scripts/tf-field-coverage.ts --update-baseline —\n" +
      "# only add to this deliberately, on review, and re-annotate the reason by\n" +
      "# hand afterwards.\n" +
      gaps.join("\n") + "\n",
  );
  console.log(`wrote ${gaps.length} entries to ${baselinePath}`);
  Deno.exit(0);
}

const newGaps = gaps.filter((g) => !baseline.has(g));
const stale = [...baseline].filter((g) => !gaps.includes(g)).sort();

console.log(
  `${paired.length} client types with a Terraform resource, ` +
    `${gaps.length} field(s) Terraform cannot express, ${baseline.size} in baseline.\n`,
);

if (newGaps.length) {
  console.log("NEW gaps — a client field with no Terraform attribute:");
  for (const g of newGaps) console.log(`  ✗ ${g}`);
  console.log(
    "\nAdd the attribute to the resource's schema and model. If it is\n" +
      "deliberately not expressible — server-derived state nobody configures, or\n" +
      "a write-only credential — run with --update-baseline and write down why.",
  );
} else {
  console.log("no new gaps ✓");
}

if (stale.length) {
  console.log(`\n${stale.length} baseline entries the provider now carries — remove them:`);
  for (const g of stale) console.log(`  · ${g}`);
}

Deno.exit(newGaps.length ? 1 : 0);
