import type {
  MediaEdge,
  MediaNode,
  NodeProductionState,
} from "../../lib/api";

const nodeTypeLabel: Record<MediaNode["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};

const statusLabel: Record<MediaNode["status"], string> = {
  draft: "草稿",
  ready: "就绪",
  queued: "排队",
  running: "运行中",
  succeeded: "完成",
  failed: "失败",
  stale: "需更新",
  user_editing: "编辑中",
};

export function AgentNodeDetailDrawer({
  edges,
  isLoading,
  node,
  nodes,
  onClose,
  productionState,
}: {
  edges: MediaEdge[];
  isLoading: boolean;
  node: MediaNode | null;
  nodes: MediaNode[];
  onClose: () => void;
  productionState: NodeProductionState | null;
}) {
  if (!node) {
    return null;
  }

  const upstream = edges
    .filter((edge) => edge.to_node_id === node.id)
    .map((edge) => nodes.find((item) => item.id === edge.from_node_id))
    .filter((item): item is MediaNode => Boolean(item));
  const downstream = edges
    .filter((edge) => edge.from_node_id === node.id)
    .map((edge) => nodes.find((item) => item.id === edge.to_node_id))
    .filter((item): item is MediaNode => Boolean(item));
  const versions = productionState?.versions ?? [];
  const latestJob = productionState?.latest_job ?? null;
  const currentVersion =
    productionState?.current_version ??
    versions.find((version) => version.winner) ??
    null;

  return (
    <aside className="agent-node-detail-drawer" aria-label="节点详情">
      <header className="agent-node-detail-header">
        <div>
          <p className="workspace-kicker">Read Only Detail</p>
          <h3>{node.title || "未命名节点"}</h3>
        </div>
        <button aria-label="关闭节点详情" onClick={onClose} type="button">
          ×
        </button>
      </header>

      <section className="agent-node-detail-section">
        <p>Status</p>
        <dl className="agent-node-detail-list">
          <DetailRow label="类型" value={nodeTypeLabel[node.node_type]} />
          <DetailRow label="状态" value={statusLabel[node.status]} />
          <DetailRow label="来源" value={node.source ?? "未知"} />
          <DetailRow label="Shot ID" value={node.shot_id ?? "未关联"} />
          <DetailRow label="Node ID" value={node.id} />
          <DetailRow label="Asset ID" value={node.asset_id ?? "无"} />
        </dl>
      </section>

      <section className="agent-node-detail-section">
        <p>Prompt</p>
        {node.prompt ? (
          <pre className="agent-node-detail-prompt">{node.prompt}</pre>
        ) : (
          <span className="agent-node-detail-empty">暂无 prompt。</span>
        )}
      </section>

      <section className="agent-node-detail-section">
        <p>Model</p>
        <dl className="agent-node-detail-list">
          <DetailRow label="Provider" value={node.model_provider ?? "未设置"} />
          <DetailRow label="Model" value={node.model_id ?? "未设置"} />
          <DetailRow
            label="Operation"
            value={node.operation_type ?? "未设置"}
          />
          <DetailRow
            label="Params"
            value={formatJSONCompact(node.model_params)}
          />
        </dl>
      </section>

      <section className="agent-node-detail-section">
        <p>Relationships</p>
        <dl className="agent-node-detail-list">
          <DetailRow label="输入依赖" value={formatNodeNames(upstream)} />
          <DetailRow label="下游节点" value={formatNodeNames(downstream)} />
        </dl>
      </section>

      <section className="agent-node-detail-section">
        <p>Shot Context</p>
        <dl className="agent-node-detail-list">
          <DetailRow label="Shot ID" value={node.shot_id ?? "未关联"} />
          <DetailRow
            label="Artifact"
            value={formatAgentArtifactKind(node.metadata)}
          />
          <DetailRow label="Canvas" value={`${node.canvas_x}, ${node.canvas_y}`} />
        </dl>
      </section>

      <section className="agent-node-detail-section">
        <p>Versions</p>
        {isLoading ? (
          <span className="agent-node-detail-empty">正在读取版本信息。</span>
        ) : versions.length > 0 ? (
          <div className="agent-node-version-list">
            {versions.slice(0, 5).map((version) => (
              <div className="agent-node-version-row" key={version.id}>
                <span>
                  v{version.version_no}
                  {version.winner ? " · winner" : ""}
                </span>
                <small>
                  {version.status}
                  {typeof version.review_score === "number"
                    ? ` · ${version.review_score}`
                    : ""}
                </small>
              </div>
            ))}
          </div>
        ) : (
          <span className="agent-node-detail-empty">暂无版本。</span>
        )}
        {currentVersion ? (
          <dl className="agent-node-detail-list">
            <DetailRow
              label="当前版本"
              value={`v${currentVersion.version_no} · ${currentVersion.status}`}
            />
          </dl>
        ) : null}
      </section>

      <section className="agent-node-detail-section">
        <p>Job</p>
        {latestJob ? (
          <dl className="agent-node-detail-list">
            <DetailRow label="Job ID" value={latestJob.id} />
            <DetailRow label="状态" value={latestJob.status} />
            <DetailRow label="模型" value={latestJob.model_id} />
            <DetailRow label="进度" value={`${latestJob.progress}%`} />
            <DetailRow
              label="重试"
              value={`${latestJob.attempt}/${latestJob.max_attempts}`}
            />
            <DetailRow label="错误" value={latestJob.error_message ?? "无"} />
          </dl>
        ) : (
          <span className="agent-node-detail-empty">暂无执行任务。</span>
        )}
      </section>

      <section className="agent-node-detail-section">
        <p>Reviews</p>
        {productionState?.review_records?.length ? (
          <div className="agent-node-version-list">
            {productionState.review_records.slice(0, 5).map((review) => (
              <div className="agent-node-version-row" key={review.id}>
                <span>
                  {review.target_phase} · {review.status}
                </span>
                <small>
                  {typeof review.overall_score === "number"
                    ? `${Math.round(review.overall_score * 100)}`
                    : "-"}
                  {` · ${review.attempt_no}/${review.max_attempts}`}
                </small>
                {review.critique ? <em>{review.critique}</em> : null}
              </div>
            ))}
          </div>
        ) : (
          <span className="agent-node-detail-empty">暂无评审记录。</span>
        )}
      </section>

      <section className="agent-node-detail-section">
        <p>Sandbox</p>
        {productionState?.sandbox_jobs?.length ? (
          <div className="agent-node-version-list">
            {productionState.sandbox_jobs.slice(0, 5).map((job) => (
              <div className="agent-node-version-row" key={job.id}>
                <span>
                  {job.operation_type || job.job_type} · {job.status}
                </span>
                <small>{job.duration_ms}ms · exit {job.exit_code ?? "-"}</small>
                {job.error_message ? <em>{job.error_message}</em> : null}
              </div>
            ))}
          </div>
        ) : (
          <span className="agent-node-detail-empty">暂无沙箱执行记录。</span>
        )}
      </section>

      <section className="agent-node-detail-section">
        <p>Final Composition</p>
        {node.operation_type === "compose_final" ||
        formatAgentArtifactKind(node.metadata) === "final_video" ? (
          <dl className="agent-node-detail-list">
            <DetailRow label="状态" value={statusLabel[node.status]} />
            <DetailRow label="当前版本" value={currentVersion?.id ?? "无"} />
            <DetailRow label="输出资源" value={node.asset_id ?? "等待生成"} />
          </dl>
        ) : (
          <span className="agent-node-detail-empty">该节点不是成片输出。</span>
        )}
      </section>

      <section className="agent-node-detail-section">
        <p>Diagnostics</p>
        {productionState?.active_stale_reasons?.length ? (
          <ul className="agent-node-detail-diagnostics">
            {productionState.active_stale_reasons.map((reason) => (
              <li key={reason.id}>{reason.reason_message}</li>
            ))}
          </ul>
        ) : (
          <span className="agent-node-detail-empty">暂无 stale 或错误信息。</span>
        )}
      </section>
    </aside>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function formatNodeNames(nodes: MediaNode[]) {
  if (nodes.length === 0) {
    return "无";
  }
  return nodes.map((node) => node.title || node.id).join("、");
}

function formatJSONCompact(value: unknown) {
  if (!value || (typeof value === "object" && Object.keys(value).length === 0)) {
    return "未设置";
  }
  try {
    return JSON.stringify(value);
  } catch {
    return "无法显示";
  }
}

function formatAgentArtifactKind(value: unknown) {
  if (!value || typeof value !== "object") {
    return "未设置";
  }
  const metadata = value as Record<string, unknown>;
  const kind = metadata.agent_artifact_kind ?? metadata.artifact_kind;
  return typeof kind === "string" && kind ? kind : "未设置";
}
