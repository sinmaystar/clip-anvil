import type { Node, NodeProps } from "@xyflow/react";
import type { CSSProperties } from "react";
import type {
  AgentWorkbenchArtifactSlot,
  AgentWorkbenchShot,
} from "../../lib/agentWorkbench";
import {
  agentWorkbenchMediaKey,
  agentWorkbenchMediaSize,
  type AgentWorkbenchMediaDimensions,
  type AgentWorkbenchMediaDimensionsByKey,
} from "../../lib/agentWorkbenchMediaLayout";
import type { AgentWorkbenchShotNodeData } from "../../lib/agentWorkbenchViewModel";
import { useAgentWorkbenchSelection } from "./AgentWorkbenchSelectionContext";

type ShotNode = Node<AgentWorkbenchShotNodeData, "agentShot">;

export function AgentShotNode({ data, selected }: NodeProps<ShotNode>) {
  const shot = data.shot;
  const artifacts = shotArtifactSlots(shot);
  const previewSlots = artifacts.filter((slot) => !isVideoSlot(slot));
  const videoSlot = artifacts.find(isVideoSlot);
  const mediaSlots = visibleMediaSlots(previewSlots, videoSlot, artifacts);
  const showReview = Boolean(shot.review || shot.issues.length > 0);
  return (
    <article
      className="agent-workbench-shot-node"
      data-selected={selected}
      data-status={shot.status}
    >
      <header>
        <div>
          <span>{shot.client_key || `#${shot.sequence_index}`}</span>
          <strong>{shot.title || "未命名分镜"}</strong>
        </div>
        <em>{shot.status}</em>
      </header>
      <p>{shot.creative_text || "等待 Producer 补充分镜描述。"}</p>
      <MediaCard
        mediaDimensions={data.mediaDimensions ?? {}}
        mediaSlots={mediaSlots}
        onMediaDimensionsChange={data.onMediaDimensionsChange}
        shot={shot}
      />
      <StatusRow
        previewCount={previewSlots.length}
        previewSlot={previewSlots[0]}
        showReview={showReview}
        shot={shot}
        videoSlot={videoSlot}
      />
    </article>
  );
}

function shotArtifactSlots(shot: AgentWorkbenchShot) {
  if (shot.artifacts && shot.artifacts.length > 0) {
    return shot.artifacts;
  }
  return [shot.preview, shot.video].filter((slot) => slot.status !== "missing");
}

function artifactSlotTitle(
  slot: AgentWorkbenchArtifactSlot,
  index: number,
  slots: AgentWorkbenchArtifactSlot[],
) {
  const sameKindIndex = slots
    .slice(0, index + 1)
    .filter((item) => item.kind === slot.kind).length;
  const sameKindTotal = slots.filter((item) => item.kind === slot.kind).length;
  const base = slot.kind === "shot_video" ? "Video" : "Preview";
  return sameKindTotal > 1 ? `${base} ${sameKindIndex}` : base;
}

function visibleMediaSlots(
  previews: AgentWorkbenchArtifactSlot[],
  video: AgentWorkbenchArtifactSlot | undefined,
  artifacts: AgentWorkbenchArtifactSlot[],
) {
  const mediaSlots = [...previews, video].filter(
    (slot): slot is AgentWorkbenchArtifactSlot => Boolean(slot),
  );
  return mediaSlots.length > 0 ? mediaSlots : artifacts.slice(0, 1);
}

function MediaCard({
  mediaDimensions,
  mediaSlots,
  onMediaDimensionsChange,
  shot,
}: {
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey;
  mediaSlots: AgentWorkbenchArtifactSlot[];
  onMediaDimensionsChange?: (
    key: string,
    dimensions: AgentWorkbenchMediaDimensions,
  ) => void;
  shot: AgentWorkbenchShot;
}) {
  return (
    <div
      className="agent-workbench-shot-media-card"
      data-empty={mediaSlots.length === 0}
      data-status={mediaSlots[0]?.status || "missing"}
    >
      <div
        className="agent-workbench-shot-media-stack"
        data-count={mediaSlots.length}
      >
        {mediaSlots.length > 0 ? (
          mediaSlots.map((slot, index) => (
            <MediaButton
              key={slot.node_id || `${slot.kind}-${index}`}
              mediaDimensions={mediaDimensions}
              onMediaDimensionsChange={onMediaDimensionsChange}
              shot={shot}
              slot={slot}
              slots={mediaSlots}
              title={artifactSlotTitle(slot, index, mediaSlots)}
            />
          ))
        ) : (
          <button
            className="agent-workbench-shot-media-button"
            data-selected={false}
            data-status="missing"
            disabled
            type="button"
          >
            <MediaPreview slot={undefined} title={shot.title || "Preview"} />
            <span className="agent-workbench-shot-media-badge">
              <strong>Preview</strong>
              <em>missing</em>
            </span>
          </button>
        )}
      </div>
    </div>
  );
}

function MediaButton({
  mediaDimensions,
  onMediaDimensionsChange,
  shot,
  slot,
  slots,
  title,
}: {
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey;
  onMediaDimensionsChange?: (
    key: string,
    dimensions: AgentWorkbenchMediaDimensions,
  ) => void;
  shot: AgentWorkbenchShot;
  slot: AgentWorkbenchArtifactSlot;
  slots: AgentWorkbenchArtifactSlot[];
  title: string;
}) {
  const selection = useAgentWorkbenchSelection();
  const mediaKey = agentWorkbenchMediaKey(slot);
  const measuredDimensions = mediaDimensions[mediaKey];
  return (
    <button
      className="agent-workbench-shot-media-button"
      data-selected={selection.isSelected("artifact", slot.node_id)}
      data-status={slot.status}
      disabled={!slot.node_id}
      onClick={(event) => {
        event.stopPropagation();
        if (!slot.node_id) {
          return;
        }
        selection.select({
          objectType: "artifact",
          objectId: slot.node_id,
          label: slot.title || title,
          });
      }}
      style={mediaSlotStyle(slot, measuredDimensions)}
      title={slot.title || title}
      type="button"
    >
      <MediaPreview
        onMediaDimensionsChange={
          onMediaDimensionsChange && mediaKey
            ? (dimensions) => onMediaDimensionsChange(mediaKey, dimensions)
            : undefined
        }
        slot={slot}
        title={shot.title || title}
      />
      <span className="agent-workbench-shot-media-badge">
        <strong>{artifactSlotTitle(slot, slots.indexOf(slot), slots)}</strong>
        <em>{slot.status}</em>
      </span>
    </button>
  );
}

function mediaSlotStyle(
  slot: AgentWorkbenchArtifactSlot,
  measuredDimensions?: AgentWorkbenchMediaDimensions,
): CSSProperties {
  const size = agentWorkbenchMediaSize(slot, measuredDimensions);
  return {
    aspectRatio: size.aspectRatio,
    height: size.height,
    width: size.width,
  };
}

function MediaPreview({
  onMediaDimensionsChange,
  slot,
  title,
}: {
  onMediaDimensionsChange?: (dimensions: AgentWorkbenchMediaDimensions) => void;
  slot: AgentWorkbenchArtifactSlot | undefined;
  title: string;
}) {
  if (!slot) {
    return <div className="agent-workbench-slot-empty">等待预览图</div>;
  }
  const image = artifactImage(slot);
  if (image && isVideoSlot(slot)) {
    return (
      <video
        controls
        draggable={false}
        muted
        playsInline
        poster={slot.thumbnail_url}
        preload="metadata"
        src={slot.access_url || slot.thumbnail_url}
      />
    );
  }
  if (image) {
    return (
      <img
        alt={slot.title || title}
        draggable={false}
        onLoad={(event) => {
          const { naturalHeight, naturalWidth } = event.currentTarget;
          if (naturalWidth <= 0 || naturalHeight <= 0) {
            return;
          }
          onMediaDimensionsChange?.({
            width: naturalWidth,
            height: naturalHeight,
          });
        }}
        src={image}
      />
    );
  }
  return (
    <div className="agent-workbench-slot-empty">
      {artifactPlaceholderText(slot.status)}
    </div>
  );
}

function StatusRow({
  previewCount,
  previewSlot,
  showReview,
  shot,
  videoSlot,
}: {
  previewCount: number;
  previewSlot: AgentWorkbenchArtifactSlot | undefined;
  showReview: boolean;
  shot: AgentWorkbenchShot;
  videoSlot: AgentWorkbenchArtifactSlot | undefined;
}) {
  return (
    <footer className="agent-workbench-shot-status-row">
      <ArtifactStatusButton
        count={previewCount}
        label="预览图"
        slot={previewSlot}
      />
      <ArtifactStatusButton label="视频" slot={videoSlot ?? shot.video} />
      {showReview ? <ReviewStatusButton shot={shot} /> : <span />}
    </footer>
  );
}

function ArtifactStatusButton({
  count,
  label,
  slot,
}: {
  count?: number;
  label: string;
  slot: AgentWorkbenchArtifactSlot | undefined;
}) {
  const selection = useAgentWorkbenchSelection();
  const status = slot?.status || "missing";
  return (
    <button
      className="agent-workbench-shot-status-pill"
      data-selected={selection.isSelected("artifact", slot?.node_id)}
      data-status={status}
      disabled={!slot?.node_id}
      onClick={(event) => {
        event.stopPropagation();
        if (!slot?.node_id) {
          return;
        }
        selection.select({
          objectType: "artifact",
          objectId: slot.node_id,
          label: slot.title || label,
        });
      }}
      type="button"
    >
      <span>{label}</span>
      <em>
        {status}
        {count && count > 1 ? ` · ${count}` : ""}
      </em>
    </button>
  );
}

function ReviewStatusButton({ shot }: { shot: AgentWorkbenchShot }) {
  const selection = useAgentWorkbenchSelection();
  return (
    <button
      className="agent-workbench-shot-status-pill"
      data-selected={selection.isSelected("review", shot.review?.id)}
      data-status={
        shot.issues.length > 0 ? "failed" : shot.review?.verdict || "none"
      }
      disabled={!shot.review}
      onClick={(event) => {
        event.stopPropagation();
        if (!shot.review) {
          return;
        }
        selection.select({
          objectType: "review",
          objectId: shot.review.id,
          label: `Review ${shot.client_key}`,
        });
      }}
      type="button"
    >
      <span>评审</span>
      <em>
        {shot.issues.length > 0
          ? `${shot.issues.length} issues`
          : shot.review?.verdict || "none"}
      </em>
    </button>
  );
}

function artifactImage(slot: AgentWorkbenchArtifactSlot | undefined) {
  return slot?.thumbnail_url || slot?.access_url || "";
}

function isVideoSlot(slot: AgentWorkbenchArtifactSlot) {
  return slot.kind === "shot_video";
}

function artifactPlaceholderText(status: string) {
  if (status === "queued") {
    return "等待生成";
  }
  if (status === "running") {
    return "生成中";
  }
  if (status === "failed") {
    return "生成失败";
  }
  if (status === "missing") {
    return "未生成";
  }
  return "暂无预览";
}
