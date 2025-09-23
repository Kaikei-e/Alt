#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

function resolveStylesheet() {
  const candidate = require("jsdom/lib/jsdom/browser/default-stylesheet.js");
  if (typeof candidate === "string" && candidate.length > 0) {
    return candidate;
  }
  if (candidate && typeof candidate.default === "string") {
    return candidate.default;
  }
  throw new Error(
    "Could not resolve jsdom default stylesheet contents as string.",
  );
}

function writeStylesheet(dest, contents) {
  mkdirSync(dirname(dest), { recursive: true });
  writeFileSync(dest, contents, "utf8");
  process.stdout.write(`[ensure-default-stylesheet] wrote ${dest}\n`);
}

const stylesheetContents = resolveStylesheet();
const destinations = [
  join(process.cwd(), ".next", "browser", "default-stylesheet.css"),
  join(
    process.cwd(),
    ".next",
    "standalone",
    ".next",
    "browser",
    "default-stylesheet.css",
  ),
];

for (const destination of destinations) {
  writeStylesheet(destination, stylesheetContents);
}

