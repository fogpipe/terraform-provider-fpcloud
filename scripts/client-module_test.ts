import { assertEquals, assertStringIncludes } from "jsr:@std/assert@1";
import {
  clientModule,
  pinnedClientDir,
  pinnedClientVersion,
} from "./client-module.ts";

// The gate reads the client this repo compiles against, not whichever release
// the module proxy has served so far (fogpipe/cloud-workspace#152).
Deno.test("the coverage gates read the client go.mod pins", () => {
  const pin = pinnedClientVersion();
  const { version, dir } = pinnedClientDir();
  assertEquals(version, pin);
  assertStringIncludes(dir, `${clientModule}@${pin}`);
  assertEquals(Deno.statSync(`${dir}/pkg/client/client.go`).isFile, true);
});
