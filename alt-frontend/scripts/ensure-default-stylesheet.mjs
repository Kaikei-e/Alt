#!/usr/bin/env node
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";

const require = createRequire(import.meta.url);

function resolveStylesheet() {
  try {
    const jsdomPath = require.resolve("jsdom");
    const jsdomDir = dirname(jsdomPath);
    const stylesheetPath = join(
      jsdomDir,
      "jsdom",
      "browser",
      "default-stylesheet.css",
    );
    const contents = readFileSync(stylesheetPath, "utf8");
    if (typeof contents === "string" && contents.length > 0) {
      return contents;
    }
    throw new Error("Stylesheet file is empty.");
  } catch (error) {
    throw new Error(
      `Could not resolve jsdom default stylesheet: ${error.message}`,
    );
  }
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
