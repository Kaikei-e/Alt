// Exercise the actual TypeScript fixture helper offline with request spies.
const { test } = require("node:test");
const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { resolve, dirname } = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");

function load(path) {
  const code = ts.transpileModule(readFileSync(path, "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText;
  const exports = {};
  const localRequire = (name) => name.startsWith(".") ? load(resolve(dirname(path), name.replace(/\.js$/, ".ts"))) : require(name);
  vm.runInThisContext(`(function(exports, require, process, Buffer) {${code}\n})`, { filename: path })(exports, localRequire, process, Buffer);
  return exports;
}
process.env.STAGING_JWT_KEY_FILE = resolve(__dirname, "../../fixtures/staging-secrets/alt_backend_token_secret.txt");
const { fixtureUserId, withSearchFixtureAuth } = load(resolve(__dirname, "../search-indexer/src/auth.ts"));
const owner = fixtureUserId("offline-owner");
const origin = "http://search-indexer:9300";
function spy() {
  const calls = [];
  const api = { get: (url, options) => calls.push({ url, options }), post: (url, options) => calls.push({ url, options }) };
  return { calls, client: withSearchFixtureAuth(api, origin) };
}

test("canonical stable UUID and bounded owner claims", () => {
  assert.match(owner, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-a[0-9a-f]{3}-[0-9a-f]{12}$/);
  assert.equal(fixtureUserId("offline-owner"), owner);
  const { calls, client } = spy();
  const url = `/v1/search?user_id=${owner}&q=rust`;
  client.get(url);
  assert.equal(calls[0].url, url);
  const token = calls[0].options.headers.Authorization.slice(7);
  const claims = JSON.parse(Buffer.from(token.split(".")[1], "base64url"));
  assert.equal(claims.sub, owner);
  assert.equal(claims.tenant_id, owner);
  assert.equal(claims.iss, "alt-staging-auth-hub");
  assert.equal(claims.aud, "alt-backend");
  assert.ok(claims.exp > Date.now() / 1000 && claims.exp <= Math.floor(Date.now() / 1000) + 300);
});
test("explicit invalid, empty, foreign and alternate proof headers remain unchanged", () => {
  for (const headers of [{ Authorization: "Bearer invalid" }, { authorization: "" }, { AUTHORIZATION: "Bearer foreign" }, { "X-Alt-Backend-Token": "invalid" }]) {
    const { calls, client } = spy();
    const options = { headers };
    client.get(`/v1/search?user_id=${owner}`, options);
    assert.equal(calls[0].options, options);
  }
});
test("non-search paths, invalid owners and foreign origins never receive credentials", () => {
  for (const url of ["/health", "/v1/search?user_id=not-uuid", `http://other:9300/v1/search?user_id=${owner}`]) {
    const { calls, client } = spy();
    client.get(url);
    assert.equal(calls[0].options.headers, undefined);
  }
});
test("Connect body, owner and explicit headers are preserved", () => {
  const { calls, client } = spy();
  const body = { userId: owner, query: "rust", limit: 1 };
  client.post("/services.search.v2.SearchService/SearchArticles", { data: body, headers: { "Content-Type": "application/json" } });
  assert.equal(calls[0].options.data, body);
  assert.equal(calls[0].options.data.userId, owner);
  assert.equal(calls[0].options.headers["Content-Type"], "application/json");
  assert.ok(calls[0].options.headers.Authorization.startsWith("Bearer "));
});
