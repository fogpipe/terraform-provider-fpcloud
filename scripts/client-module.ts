// The github.com/fogpipe/cloud-cli source both coverage gates read, resolved
// at the version go.mod pins and nowhere else.
//
// They used to resolve `cloud-cli@latest`, so the yardstick was whatever the
// module proxy had served so far: proxy.golang.org learns of a tag lazily and
// serves a stale @latest for up to 30 minutes after — or until somebody asks
// for the new version by name — and the verdict changed with no change to
// this repo. On 2026-09-05 a three-line docs commit turned the resource gate
// red on `AuthConfig`, a method the pin did not carry, because the proxy had
// moved under it; the last green run had judged the same tree against an
// older client (fogpipe/cloud-workspace#152). A gate whose input is not
// determined by the commit reports a repo covered when it is not, and fails on
// whoever pushes next.
//
// The pin is exact: a version resolves to one module zip, checked against
// go.sum, so the reading is the same here, in CI and next week, and a gap is
// red on the bump that introduces it. A pin lagging the released client is a
// different question and `just release` asks it (docs/release-policy.md).
//
// GOWORK=off so a session tree resolves the pinned module rather than the
// sibling cloud-cli checkout: this repo builds alone everywhere else.

export const clientModule = "github.com/fogpipe/cloud-cli";

const root = new URL("../", import.meta.url).pathname;

const go = (...args: string[]) => {
  const out = new Deno.Command("go", {
    args,
    cwd: root,
    env: { ...Deno.env.toObject(), GOWORK: "off" },
  }).outputSync();
  if (!out.success) {
    throw new Error(
      `go ${args.join(" ")} failed: ${new TextDecoder().decode(out.stderr)}`,
    );
  }
  return new TextDecoder().decode(out.stdout).trim();
};

export const pinnedClientVersion = () => {
  const line = Deno.readTextFileSync(`${root}go.mod`).split("\n")
    .map((l) => l.trim())
    .find((l) => l.startsWith(`${clientModule} `));
  if (!line) throw new Error(`go.mod does not require ${clientModule}`);
  return line.split(/\s+/)[1];
};

// The directory holding the pinned client's source. `go list -m` answers with
// an empty Dir — not an error — when the module is known but not yet
// downloaded, the normal state of a fresh CI checkout, so the download is
// forced and an empty answer after it is refused rather than read as a client
// with no methods.
//
// Version and Dir are asked for separately: printed on one line, an empty Dir
// leaves nothing after the version, `go()` trims the line, and the version
// itself was read as the directory — so the download branch below could never
// run, and the first pin bump on a fresh cache failed the gate on a module it
// had been told about and never fetched (fogpipe/cloud-workspace#297).
export const pinnedClientDir = () => {
  const want = pinnedClientVersion();
  const version = go("list", "-m", "-f", "{{.Version}}", clientModule);
  const dir = go("list", "-m", "-f", "{{.Dir}}", clientModule);
  if (version !== want) {
    throw new Error(
      `go.mod pins ${clientModule} ${want} but the build list resolves ${version}`,
    );
  }
  if (dir !== "") return { version, dir };
  go("mod", "download", `${clientModule}@${want}`);
  const again = go("list", "-m", "-f", "{{.Dir}}", clientModule);
  if (again === "") {
    throw new Error(
      `${clientModule}@${want} is not in the module cache after go mod download`,
    );
  }
  return { version, dir: again };
};
