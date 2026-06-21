import type { ArtifactVersion } from "./api";

export interface ArtifactViewSource {
  accessUrl?: string;
  text?: string;
  mime?: string;
  type?: string;
}

export function openArtifactVersionInNewTab(version: ArtifactVersion) {
  return openArtifactViewInNewTab({
    accessUrl: version.asset?.access_url,
    text: version.asset?.text_content,
    mime: version.asset?.mime,
    type: version.asset?.type,
  });
}

export function openArtifactViewInNewTab(source: ArtifactViewSource) {
  const html = renderArtifactViewerHTML(source);
  if (!html) {
    return false;
  }
  const url = URL.createObjectURL(
    new Blob([html], { type: "text/html;charset=utf-8" }),
  );
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
  window.open(url, "_blank", "noopener,noreferrer");
  return true;
}

export function renderArtifactViewerHTML(source: ArtifactViewSource) {
  const body = renderArtifactViewerBody(source);
  if (!body) {
    return "";
  }
  return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>ClipAnvil 全屏查看</title>
<style>
html,
body {
  margin: 0;
  min-height: 100%;
  background: #111827;
  color: #f8fafc;
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
body {
  display: grid;
  place-items: center;
}
img,
video {
  display: block;
  max-width: 100vw;
  max-height: 100vh;
}
audio {
  width: min(720px, calc(100vw - 48px));
}
pre {
  box-sizing: border-box;
  width: min(960px, calc(100vw - 48px));
  max-height: calc(100vh - 48px);
  margin: 24px;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.65;
}
.empty {
  color: #94a3b8;
}
</style>
</head>
<body>
${body}
</body>
</html>`;
}

function renderArtifactViewerBody(source: ArtifactViewSource) {
  const kind = source.type || source.mime?.split("/")[0] || "";
  if (source.accessUrl && kind === "image") {
    return `<img alt="ClipAnvil asset" src="${escapeAttribute(source.accessUrl)}" />`;
  }
  if (source.accessUrl && kind === "video") {
    return `<video controls src="${escapeAttribute(source.accessUrl)}"></video>`;
  }
  if (source.accessUrl && kind === "audio") {
    return `<audio controls src="${escapeAttribute(source.accessUrl)}"></audio>`;
  }
  if (typeof source.text === "string") {
    return `<pre>${escapeHTML(source.text)}</pre>`;
  }
  if (source.accessUrl) {
    return `<iframe title="ClipAnvil asset" src="${escapeAttribute(source.accessUrl)}"></iframe>`;
  }
  return "";
}

function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeAttribute(value: string) {
  return escapeHTML(value).replace(/"/g, "&quot;");
}
