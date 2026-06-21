import type {
  ArtifactVersion,
  GenerationJob,
  MediaNode,
  StaleReason,
} from "./api";
import { referencePackSummaryText } from "./referencePack.js";

export function winnerPreviewText(
  node: Pick<
    MediaNode,
    "node_type" | "prompt" | "production_preview" | "reference_pack_preview"
  >,
) {
  const preview = node.production_preview;
  if (node.node_type === "text" && preview?.text) {
    return preview.text;
  }
  if (node.node_type === "reference_pack") {
    return referencePackSummaryText(node.reference_pack_preview);
  }
  if (preview?.asset_type) {
    return `${preview.asset_type} v${preview.version_no}`;
  }
  return node.prompt || "等待输入 prompt";
}

export function formatInputHash(hash?: string) {
  if (!hash) {
    return "-";
  }
  return hash.replace(/^sha256:/, "").slice(0, 8);
}

export function versionRows(versions: ArtifactVersion[]) {
  return [...versions]
    .sort((a, b) => b.version_no - a.version_no)
    .map((version) => ({
      id: version.id,
      versionLabel: `v${version.version_no}`,
      isWinner: version.winner,
      status: version.status,
      statusLabel: versionStatusLabel(version),
      progressLabel: versionProgressLabel(version),
      assetLabel: version.asset?.type ?? "output",
      assetDetail: artifactAssetDetail(version),
      canSelect:
        version.status === "succeeded" && Boolean(version.asset) && !version.winner,
      inputHash: formatInputHash(version.input_hash),
      error: version.error_message || version.error_code || "",
      createdAt: version.created_at,
    }));
}

export function versionStatusLabel(
  version: Pick<ArtifactVersion, "status" | "winner" | "progress">,
) {
  if (version.winner && version.status === "succeeded") {
    return "current";
  }
  switch (version.status) {
    case "queued":
      return "queued";
    case "running":
      return "running";
    case "failed":
      return "failed";
    case "cancelled":
      return "cancelled";
    case "succeeded":
      return "ready";
    default:
      return "pending";
  }
}

export function versionProgressLabel(
  version: Pick<ArtifactVersion, "status" | "progress">,
) {
  if (version.status !== "queued" && version.status !== "running") {
    return "";
  }
  return `${Math.max(0, Math.min(100, Math.round(version.progress ?? 0)))}%`;
}

export function versionDetailRows(version: ArtifactVersion) {
  const rows = [
    { label: "Status", value: version.status },
    {
      label: "Version",
      value: `v${version.version_no}${version.winner ? " · current" : ""}`,
    },
    { label: "Asset", value: artifactAssetDetail(version) },
    { label: "Input Hash", value: formatInputHash(version.input_hash) },
  ];
  if (version.job_id) {
    rows.push({ label: "Job", value: version.job_id });
  }
  if (version.started_at) {
    rows.push({ label: "Started", value: version.started_at });
  }
  if (version.completed_at) {
    rows.push({ label: "Completed", value: version.completed_at });
  }
  return rows;
}

export function versionHasCallRecord(version: ArtifactVersion) {
  return (
    Boolean(version.error_code || version.error_message) ||
    hasObjectEntries(version.provider_request) ||
    hasObjectEntries(version.provider_response)
  );
}

export function versionCallRecordBlocks(version: ArtifactVersion) {
  const blocks: Array<{ label: string; value: unknown }> = [];
  if (version.error_code || version.error_message) {
    blocks.push({
      label: "Error",
      value:
        version.error_code && version.error_message
          ? `${version.error_code}: ${version.error_message}`
          : version.error_message || version.error_code,
    });
  }
  if (hasObjectEntries(version.provider_request)) {
    blocks.push({ label: "Provider Request", value: version.provider_request });
  }
  if (hasObjectEntries(version.provider_response)) {
    blocks.push({
      label: "Provider Response",
      value: version.provider_response,
    });
  }
  return blocks;
}

function artifactAssetDetail(version: ArtifactVersion) {
  if (version.status === "queued" || version.status === "running") {
    return versionProgressLabel(version) || "等待 provider";
  }
  if (version.status === "failed") {
    return version.error_message || version.error_code || "运行失败";
  }
  const parts = [];
  if (version.asset?.mime) {
    parts.push(version.asset.mime);
  }
  const width = numberMetadata(version.asset?.metadata?.width);
  const height = numberMetadata(version.asset?.metadata?.height);
  if (width && height) {
    parts.push(`${width}x${height}`);
  }
  if (version.asset?.size_bytes) {
    parts.push(formatBytes(version.asset.size_bytes));
  }
  return parts.join(" · ") || "-";
}

function hasObjectEntries(value?: Record<string, unknown>) {
  return Boolean(value && Object.keys(value).length > 0);
}

function numberMetadata(value: unknown) {
  return typeof value === "number" && value > 0 ? Math.round(value) : 0;
}

function formatBytes(value: number) {
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${Math.round(value / 1024)} KB`;
  }
  return `${Math.round(value / 1024 / 1024)} MB`;
}

export function jobDetailRows(
  job: Partial<
    Pick<
      GenerationJob,
      | "status"
      | "operation_type"
      | "provider"
      | "model_id"
      | "rendered_prompt"
      | "attempt"
      | "max_attempts"
      | "error_code"
      | "error_message"
    >
  >,
) {
  const error =
    job.error_code && job.error_message
      ? `${job.error_code}: ${job.error_message}`
      : job.error_message || job.error_code || "";

  return [
    { label: "Status", value: job.status ?? "-" },
    { label: "Operation", value: job.operation_type ?? "-" },
    { label: "Model", value: `${job.provider ?? "-"}/${job.model_id ?? "-"}` },
    {
      label: "Attempt",
      value:
        typeof job.attempt === "number" && typeof job.max_attempts === "number"
          ? `Attempt ${job.attempt} / ${job.max_attempts}`
          : "-",
    },
    { label: "Prompt", value: job.rendered_prompt || "-" },
    { label: "Error", value: error || "-" },
  ];
}

export function staleReasonText(
  reason: Pick<
    StaleReason,
    | "reason_code"
    | "reason_message"
    | "upstream_node_id"
    | "upstream_version_id"
    | "details"
  >,
) {
  const upstream = reason.upstream_version_id
    ? `${reason.upstream_node_id} -> ${reason.upstream_version_id}`
    : reason.upstream_node_id;
  return `${reason.reason_message || reason.reason_code} (${upstream})`;
}
