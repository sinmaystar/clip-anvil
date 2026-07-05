#!/usr/bin/env node

import {readFile} from "node:fs/promises";

const forbiddenPatterns = [
  ["dynamic_import", /import\s*\(/],
  ["require_call", /require\s*\(/],
  ["network_api", /\b(fetch|XMLHttpRequest|WebSocket)\b/],
  ["eval_call", /\beval\s*\(/],
  ["function_constructor", /\bnew\s+Function\b|\bFunction\s*\(/],
  ["external_url", /https?:\/\//],
];

export function validateSourceText(file, text) {
  const errors = [];
  const lines = String(text).split(/\r?\n/);
  for (let i = 0; i < lines.length; i += 1) {
    for (const [code, pattern] of forbiddenPatterns) {
      if (pattern.test(lines[i])) {
        errors.push({severity: "error", code, file, line: i + 1});
      }
    }
  }
  return {passed: errors.length === 0, errors};
}

export async function validateFile(file) {
  const text = await readFile(file, "utf8");
  return validateSourceText(file, text);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const file = process.argv[2];
  if (!file) {
    console.error("usage: validate.mjs <file>");
    process.exit(2);
  }
  const result = await validateFile(file);
  console.log(JSON.stringify(result));
  process.exit(result.passed ? 0 : 1);
}
