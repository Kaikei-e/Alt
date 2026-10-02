import { assert } from "jsr:@std/assert";
import { generateCryptoRandomPassword } from "./feed-load-test-setup.ts";

Deno.test("setup password generation", () => {
  const pwd = generateCryptoRandomPassword();
  assert(pwd.length > 10);
});
