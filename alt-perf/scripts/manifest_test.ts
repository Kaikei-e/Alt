import { assertEquals } from "jsr:@std/assert";
import { isMockRssUrl, isSyntheticEmail, isValidUUID } from "./manifest.ts";

Deno.test("manifest validation helpers", () => {
  assertEquals(isValidUUID("11111111-1111-1111-1111-111111111111"), true);
  assertEquals(isValidUUID("invalid-uuid"), false);

  assertEquals(isSyntheticEmail("loadtest-123@test.alt.local"), true);
  assertEquals(isSyntheticEmail("test@example.com"), false);

  assertEquals(isMockRssUrl("http://mock-rss-0:8080/feeds/1/rss.xml"), true);
  assertEquals(isMockRssUrl("http://evil.com/rss.xml"), false);
});
