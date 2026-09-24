import { describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";

interface ExtractedImport {
  line: number;
  specifier: string;
  statement: string;
}

/**
 * Recursively collects all TypeScript/JavaScript files in a layer directory.
 * Asserts that at least one file exists in the layer.
 */
async function collectSourceFiles(layer: string): Promise<URL[]> {
  const dirUrl = new URL(`../../src/${layer}/`, import.meta.url);
  const files: URL[] = [];

  async function walk(current: URL) {
    for await (const entry of Deno.readDir(current)) {
      const entryUrl = new URL(
        entry.name + (entry.isDirectory ? "/" : ""),
        current,
      );
      if (entry.isDirectory) {
        await walk(entryUrl);
      } else if (
        entry.isFile &&
        (entry.name.endsWith(".ts") || entry.name.endsWith(".js"))
      ) {
        files.push(entryUrl);
      }
    }
  }

  await walk(dirUrl);
  assertEquals(
    files.length > 0,
    true,
    `Layer '${layer}' must contain at least one source file`,
  );
  return files;
}

/**
 * Extracts import specifiers from file content, handling:
 * - import ... from "..."
 * - import type ... from "..." (including multi-line imports)
 * - export ... from "..."
 * - bare import "..."
 * - dynamic import("...")
 */
function extractImports(source: string): ExtractedImport[] {
  const imports: ExtractedImport[] = [];
  const lines = source.split("\n");

  // Scan whole-file `from "..."` specifiers (covers single-line and multi-line imports)
  const fromRegex = /\bfrom\s*["']([^"']+)["']/g;
  let match: RegExpExecArray | null;
  while ((match = fromRegex.exec(source)) !== null) {
    const prefix = source.substring(0, match.index);
    const line = prefix.split("\n").length;
    const statement = lines[line - 1] ?? match[0];
    imports.push({
      line,
      specifier: match[1]!,
      statement: statement.trim(),
    });
  }

  // Bare side-effect imports: import "..."
  const bareImportRegex = /(?:^|\n)\s*import\s*["']([^"']+)["']/g;
  while ((match = bareImportRegex.exec(source)) !== null) {
    const prefix = source.substring(
      0,
      match.index + (match[0].startsWith("\n") ? 1 : 0),
    );
    const line = prefix.split("\n").length;
    const statement = lines[line - 1] ?? match[0];
    imports.push({
      line,
      specifier: match[1]!,
      statement: statement.trim(),
    });
  }

  // Dynamic import("...")
  const dynamicImportRegex = /\bimport\s*\(\s*["']([^"']+)["']\s*\)/g;
  while ((match = dynamicImportRegex.exec(source)) !== null) {
    const prefix = source.substring(0, match.index);
    const line = prefix.split("\n").length;
    const statement = lines[line - 1] ?? match[0];
    imports.push({
      line,
      specifier: match[1]!,
      statement: statement.trim(),
    });
  }

  return imports;
}

/**
 * Returns true if the specifier targets the given layer name.
 */
function specifierTargetsLayer(
  specifier: string,
  targetLayer: string,
): boolean {
  return new RegExp(`(?:^|[\\/])${targetLayer}(?:[\\/]|\\.ts$|$)`).test(
    specifier,
  );
}

/**
 * Scans all source files in a layer and checks for forbidden import targets.
 */
async function checkLayerForbiddenImports(
  layer: string,
  forbiddenLayers: string[],
): Promise<string[]> {
  const files = await collectSourceFiles(layer);
  const violations: string[] = [];

  for (const fileUrl of files) {
    const content = await Deno.readTextFile(fileUrl);
    const imports = extractImports(content);
    const relPath = fileUrl.pathname.replace(/^.*\/src\//, "src/");

    for (const imp of imports) {
      for (const forbidden of forbiddenLayers) {
        if (specifierTargetsLayer(imp.specifier, forbidden)) {
          violations.push(
            `${relPath}:${imp.line}: forbidden import of '${forbidden}' layer via '${imp.specifier}' (${imp.statement})`,
          );
        }
      }
    }
  }

  return violations;
}

describe("Clean Architecture layer boundaries", () => {
  it("domain layer must not define port interfaces with methods", async () => {
    const domainFiles = await collectSourceFiles("domain");
    for (const fileUrl of domainFiles) {
      const content = await Deno.readTextFile(fileUrl);
      const hasSecretManager = /export\s+interface\s+SecretManager\s*\{/.test(
        content,
      );
      assertEquals(
        hasSecretManager,
        false,
        "SecretManager port interface must be defined in port layer, not domain layer",
      );
    }
  });

  it("gateway layer must not import port interfaces from domain", async () => {
    const gatewayFiles = await collectSourceFiles("gateway");
    const violations: string[] = [];

    for (const fileUrl of gatewayFiles) {
      const content = await Deno.readTextFile(fileUrl);
      const imports = extractImports(content);
      const relPath = fileUrl.pathname.replace(/^.*\/src\//, "src/");

      for (const imp of imports) {
        if (
          specifierTargetsLayer(imp.specifier, "domain") &&
          /import\s*\{[^}]*\bSecretManager\b[^}]*\}\s*from/.test(content)
        ) {
          violations.push(
            `${relPath}:${imp.line}: imports SecretManager from domain`,
          );
        }
      }
    }

    assertEquals(
      violations,
      [],
      `Gateway layer must import SecretManager from port layer, not domain: ${
        violations.join(", ")
      }`,
    );
  });

  it("domain layer must not import port/usecase/gateway/handler", async () => {
    const violations = await checkLayerForbiddenImports("domain", [
      "port",
      "usecase",
      "gateway",
      "handler",
    ]);
    assertEquals(
      violations,
      [],
      `Domain layer violations:\n${violations.join("\n")}`,
    );
  });

  it("usecase layer must not import gateway/handler", async () => {
    const violations = await checkLayerForbiddenImports("usecase", [
      "gateway",
      "handler",
    ]);
    assertEquals(
      violations,
      [],
      `Usecase layer violations:\n${violations.join("\n")}`,
    );
  });

  it("port layer must not import gateway/handler/usecase", async () => {
    const violations = await checkLayerForbiddenImports("port", [
      "gateway",
      "handler",
      "usecase",
    ]);
    assertEquals(
      violations,
      [],
      `Port layer violations:\n${violations.join("\n")}`,
    );
  });

  it("handler layer must not import port/gateway", async () => {
    const violations = await checkLayerForbiddenImports("handler", [
      "port",
      "gateway",
    ]);
    assertEquals(
      violations,
      [],
      `Handler layer violations:\n${violations.join("\n")}`,
    );
  });
});
