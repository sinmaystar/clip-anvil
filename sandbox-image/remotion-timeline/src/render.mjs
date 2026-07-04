import {spawn} from "node:child_process";
import {readFile} from "node:fs/promises";
import {fileURLToPath} from "node:url";

const args = parseArgs(process.argv.slice(2));

if (!args.props || !args.out) {
  console.error("Usage: node render.mjs --props <timeline-plan.json> --out <output.mp4> [--browser-executable <path>]");
  process.exit(2);
}

const raw = await readFile(args.props, "utf8");
const plan = JSON.parse(raw);
assertTimelinePlan(plan);

const remotionBin = fileURLToPath(new URL("../node_modules/.bin/remotion", import.meta.url));
const entryPoint = fileURLToPath(new URL("./index.tsx", import.meta.url));
const renderArgs = [
  "render",
  entryPoint,
  "MarketingTimeline",
  args.out,
  `--props=${args.props}`,
  "--codec=h264",
  `--public-dir=${args.publicDir || "/workspace"}`,
  "--overwrite"
];

if (args.browserExecutable) {
  renderArgs.push(`--browser-executable=${args.browserExecutable}`);
}

await run(remotionBin, renderArgs);

function parseArgs(values) {
  const out = {};
  for (let index = 0; index < values.length; index++) {
    const item = values[index];
    if (item === "--props") {
      out.props = values[++index];
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

function assertTimelinePlan(value) {
  if (!value || value.schema !== "clipanvil.remotion_timeline.v1") {
    throw new Error("Invalid timeline schema");
  }
  if (value.composition !== "MarketingTimeline") {
    throw new Error("Invalid timeline composition");
  }
  if (!value.output || value.output.width <= 0 || value.output.height <= 0 || value.output.fps <= 0 || value.output.duration_sec <= 0) {
    throw new Error("Invalid timeline output");
  }
  if (!Array.isArray(value.segments) || value.segments.length === 0) {
    throw new Error("Timeline requires at least one segment");
  }
  for (const segment of value.segments) {
    if (!segment.id || segment.end_sec <= segment.start_sec || !Array.isArray(segment.assets) || segment.assets.length === 0) {
      throw new Error(`Invalid segment ${segment.id || "unknown"}`);
    }
    for (const asset of segment.assets) {
      if (asset.type !== "image" && asset.type !== "video") {
        throw new Error(`Invalid asset type ${asset.type}`);
      }
      assertWorkspacePath(asset.workspace_path);
    }
  }
  for (const track of value.audio_tracks || []) {
    if (track.role !== "voiceover" && track.role !== "bgm") {
      throw new Error(`Invalid audio role ${track.role}`);
    }
    assertWorkspacePath(track.workspace_path);
  }
}

function assertWorkspacePath(value) {
  if (!value || !value.startsWith("/workspace/")) {
    throw new Error(`Path must be under /workspace: ${value}`);
  }
}

function run(command, commandArgs) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, commandArgs, {stdio: "inherit"});
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${command} exited with code ${code}`));
    });
  });
}
