import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { relative, resolve } from "node:path";
import { describe, it } from "node:test";
import { URL } from "node:url";

const repoRoot = resolve(new URL("../../../../", import.meta.url).pathname);
const codeRoots = ["apps/web", "packages"];
const currentDocRoots = [
  "AGENTS.md",
  "CLAUDE.md",
  "docs/README.md",
  "docs/engineering",
  "docs/design",
];
const self = relative(repoRoot, new URL(import.meta.url).pathname);

describe("canvas runtime removal", () => {
  it("keeps frontend and package code free of the retired canvas runtime", () => {
    const retiredRuntime = ["tl", "draw"].join("");
    const retiredPackage = ["@tl", "draw"].join("");
    const retiredRecord = ["TL", "Record"].join("");
    const retiredShape = ["TL", "Shape"].join("");
    const retiredUtil = ["Shape", "Util"].join("");
    const matches = collectMatches(codeRoots, new RegExp(`\\b${retiredRuntime}\\b|${retiredPackage}|${retiredRecord}|${retiredShape}|${retiredUtil}`, "gi"), [
      self,
    ]);

    assert.deepEqual(matches, []);
  });

  it("keeps current docs on the React Flow canvas contract", () => {
    const retiredRuntime = ["tl", "draw"].join("");
    const matches = collectMatches(
      currentDocRoots,
      new RegExp(`\\b${retiredRuntime}\\b`, "gi"),
    );

    assert.deepEqual(matches, []);
  });
});

function collectMatches(paths, pattern, ignoredRelativePaths = []) {
  const ignored = new Set(ignoredRelativePaths);
  const matches = [];

  for (const path of paths) {
    const absolutePath = resolve(repoRoot, path);
    for (const filePath of walkFiles(absolutePath)) {
      const relativePath = relative(repoRoot, filePath);
      if (ignored.has(relativePath) || shouldSkip(relativePath)) {
        continue;
      }
      const text = readFileSync(filePath, "utf8");
      const lines = text.split("\n");
      lines.forEach((line, index) => {
        pattern.lastIndex = 0;
        if (pattern.test(line)) {
          matches.push(`${relativePath}:${index + 1}:${line.trim()}`);
        }
      });
    }
  }

  return matches;
}

function* walkFiles(path) {
  const stat = statSync(path);
  if (stat.isFile()) {
    yield path;
    return;
  }

  for (const entry of readdirSync(path, { withFileTypes: true })) {
    const child = resolve(path, entry.name);
    if (entry.isDirectory()) {
      yield* walkFiles(child);
      continue;
    }
    if (entry.isFile()) {
      yield child;
    }
  }
}

function shouldSkip(relativePath) {
  return (
    relativePath.includes("/node_modules/") ||
    relativePath.includes("/dist/") ||
    relativePath.includes("/.vite/") ||
    relativePath.endsWith(".tsbuildinfo") ||
    relativePath.endsWith(".png") ||
    relativePath.endsWith(".jpg") ||
    relativePath.endsWith(".jpeg") ||
    relativePath.endsWith(".webp")
  );
}
