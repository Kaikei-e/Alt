import { describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";

const read = (relative: string) =>
  Deno.readTextFileSync(new URL(relative, import.meta.url));

function permission(source: string, flag: string): string[] {
  const value = source.match(new RegExp(`--${flag}=([^\\s"]+)`))?.[1];
  if (!value) throw new Error(`--${flag} not found`);
  return value.split(",").sort();
}

// deno.json's start task is what the tests and a bare-host run use; the image
// carries its own copy of the same flags. A name the code reads that only one
// of them lists is an uncaught NotCapable at container start.
describe("container permission flags", () => {
  const start = JSON.parse(read("../../../deno.json")).tasks.start as string;
  const entrypoint = read("../../../Dockerfile").split("ENTRYPOINT")[1] ?? "";

  for (const flag of ["allow-env", "allow-net", "allow-read", "allow-write"]) {
    it(`ENTRYPOINT --${flag} matches the start task`, () => {
      assertEquals(permission(entrypoint, flag), permission(start, flag));
    });
  }
});
