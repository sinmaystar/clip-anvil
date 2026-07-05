import {spawn} from "node:child_process";
import {cp, lstat, mkdir, readFile, readdir, rm, symlink, writeFile} from "node:fs/promises";
import {basename, dirname, extname, join, relative, resolve, sep} from "node:path";
import {fileURLToPath} from "node:url";

const args = parseArgs(process.argv.slice(2));

if (!args.workdir || !args.out) {
  console.error("Usage: node render.mjs --workdir <attempt-dir> --out <output.mp4> [--browser-executable <path>] [--public-dir <path>]");
  process.exit(2);
}

const workdir = assertInside("/workspace/agent-remotion", args.workdir, "workdir");
const outputPath = assertInside("/workspace/output", args.out, "out");
const publicDir = args.publicDir ? assertInside("/workspace", args.publicDir, "public-dir") : "/workspace";
const propsPath = join(workdir, "props.json");
const generatedEntryPath = join(workdir, "GeneratedComposition.tsx");

await assertFile(generatedEntryPath, "GeneratedComposition.tsx is required");
const props = JSON.parse(await readFile(propsPath, "utf8"));
assertProps(props);

const runtimeDir = join(workdir, ".clipanvil-runtime");
const generatedDir = join(runtimeDir, "generated");
const runtimeHelperDir = join(runtimeDir, "runtime");
await rm(runtimeDir, {recursive: true, force: true});
await mkdir(generatedDir, {recursive: true});
await mkdir(runtimeHelperDir, {recursive: true});

await copyGeneratedFiles(workdir, generatedDir);
await writeFile(join(runtimeHelperDir, "safe.ts"), safeHelperSource(), "utf8");
await writeFile(join(runtimeDir, "harness.tsx"), harnessSource(), "utf8");
await linkNodeModules(runtimeDir);
await preparePublicAssetBridge(publicDir);

const remotionBin = fileURLToPath(new URL("../node_modules/.bin/remotion", import.meta.url));
const renderArgs = [
  "render",
  join(runtimeDir, "harness.tsx"),
  "AgentGeneratedVideo",
  outputPath,
  `--props=${propsPath}`,
  "--codec=h264",
  `--public-dir=${publicDir}`,
  "--overwrite"
];

if (args.browserExecutable) {
  renderArgs.push(`--browser-executable=${args.browserExecutable}`);
}

await run(remotionBin, renderArgs, runtimeDir);

function parseArgs(values) {
  const out = {};
  for (let index = 0; index < values.length; index += 1) {
    const item = values[index];
    if (item === "--workdir") {
      out.workdir = values[++index];
    } else if (item === "--out") {
      out.out = values[++index];
    } else if (item === "--browser-executable") {
      out.browserExecutable = values[++index];
    } else if (item === "--public-dir") {
      out.publicDir = values[++index];
    }
  }
  return out;
}

function assertInside(root, value, label) {
  const resolvedRoot = resolve(root);
  const resolvedValue = resolve(value || "");
  if (resolvedValue !== resolvedRoot && !resolvedValue.startsWith(`${resolvedRoot}${sep}`)) {
    throw new Error(`${label} must be inside ${root}: ${value}`);
  }
  return resolvedValue;
}

async function assertFile(file, message) {
  const info = await lstat(file);
  if (!info.isFile()) {
    throw new Error(message);
  }
}

function assertProps(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("props.json must contain an object");
  }
  const output = value.output;
  if (!output || output.width <= 0 || output.height <= 0 || output.fps <= 0 || output.duration_sec <= 0) {
    throw new Error("props.output requires positive width, height, fps, and duration_sec");
  }
}

async function copyGeneratedFiles(sourceDir, generatedDir) {
  const entries = await collectGeneratedFiles(sourceDir);
  for (const sourcePath of entries) {
    const rel = relative(sourceDir, sourcePath);
    const dest = join(generatedDir, rel);
    await mkdir(dirname(dest), {recursive: true});
    await cp(sourcePath, dest);
  }
}

async function collectGeneratedFiles(sourceDir) {
  const result = [];
  const stack = [sourceDir];
  while (stack.length > 0) {
    const current = stack.pop();
    const entries = await import("node:fs/promises").then(({readdir}) => readdir(current, {withFileTypes: true}));
    for (const entry of entries) {
      if (entry.name.startsWith(".")) {
        continue;
      }
      if (entry.name === "props.json" || entry.name === "node_modules") {
        continue;
      }
      const fullPath = join(current, entry.name);
      if (entry.isDirectory()) {
        stack.push(fullPath);
        continue;
      }
      if (!entry.isFile()) {
        continue;
      }
      if (![".ts", ".tsx", ".json"].includes(extname(entry.name))) {
        continue;
      }
      result.push(fullPath);
    }
  }
  result.sort();
  return result;
}

async function preparePublicAssetBridge(publicDir) {
  const inputDir = "/workspace/input";
  let entries = [];
  try {
    entries = await readdir(inputDir, {withFileTypes: true});
  } catch {
    return;
  }

  await mkdir(publicDir, {recursive: true});
  await linkInputEntries(entries, inputDir, publicDir);

  const nestedPublicDir = join(publicDir, "public");
  await mkdir(nestedPublicDir, {recursive: true});
  await linkInputEntries(entries, inputDir, nestedPublicDir);
  await linkIfMissing(inputDir, join(nestedPublicDir, "input"), "dir");
}

async function linkInputEntries(entries, inputDir, targetDir) {
  for (const entry of entries) {
    if (!entry.isFile()) {
      continue;
    }
    await linkIfMissing(join(inputDir, entry.name), join(targetDir, entry.name), "file");
  }
}

async function linkIfMissing(source, dest, type) {
  try {
    await lstat(dest);
    return;
  } catch {
    // Missing destination; create it below.
  }
  try {
    await symlink(source, dest, type);
  } catch {
    await cp(source, dest, {recursive: type === "dir"});
  }
}

async function linkNodeModules(runtimeDir) {
  const packageRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
  const nodeModules = join(packageRoot, "node_modules");
  await symlink(nodeModules, join(runtimeDir, "node_modules"), "dir");
}

function harnessSource() {
  return `import React from "react";
import {Composition, registerRoot} from "remotion";
import * as GeneratedModule from "./generated/GeneratedComposition";

const GeneratedComponent = (GeneratedModule as any).AgentGeneratedComposition || (GeneratedModule as any).default;

if (!GeneratedComponent) {
  throw new Error("GeneratedComposition.tsx must export default or AgentGeneratedComposition");
}

const defaultProps = {
  output: {width: 1080, height: 1920, fps: 30, duration_sec: 6}
};

const Root: React.FC = () => (
  <Composition
    id="AgentGeneratedVideo"
    component={GeneratedComponent}
    defaultProps={defaultProps}
    durationInFrames={180}
    fps={30}
    width={1080}
    height={1920}
    calculateMetadata={({props}) => {
      const output = (props as any)?.output || defaultProps.output;
      const fps = Number(output.fps || 30);
      const durationSec = Number(output.duration_sec || 6);
      return {
        width: Number(output.width || 1080),
        height: Number(output.height || 1920),
        fps,
        durationInFrames: Math.max(1, Math.round(durationSec * fps)),
      };
    }}
  />
);

registerRoot(Root);
`;
}

function safeHelperSource() {
  return `import {staticFile} from "remotion";

export function assetPath(value: string): string {
  if (!value || value.includes("..") || value.startsWith("http://") || value.startsWith("https://")) {
    throw new Error("unsafe asset path");
  }
  return staticFile(value.replace(/^\\/workspace\\/?/, "").replace(/^\\//, ""));
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
`;
}

function run(command, commandArgs, cwd) {
  return new Promise((resolveRun, reject) => {
    const child = spawn(command, commandArgs, {stdio: "inherit", cwd});
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolveRun();
        return;
      }
      reject(new Error(`${basename(command)} exited with code ${code}`));
    });
  });
}
