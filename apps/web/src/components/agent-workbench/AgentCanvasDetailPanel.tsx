import type { ReactNode } from "react";
import type {
  AgentCanvasArtifactDetail,
  AgentCanvasArtifactSlot,
  AgentCanvasDetail,
  AgentCanvasIssueSummary,
  AgentCanvasReviewRecord,
  AgentCanvasShotDetail,
  AgentJsonValue,
} from "../../lib/agentApi";
import type { AgentWorkbenchSelection } from "../../lib/agentWorkbenchSelection";

interface AgentCanvasDetailPanelProps {
  detail?: AgentCanvasDetail;
  error?: Error | null;
  isLoading: boolean;
  selection: AgentWorkbenchSelection | null;
  onClose: () => void;
  onRetry: () => void;
  onSelectObject: (selection: AgentWorkbenchSelection) => void;
}

export function AgentCanvasDetailPanel({
  detail,
  error,
  isLoading,
  selection,
  onClose,
  onRetry,
  onSelectObject,
}: AgentCanvasDetailPanelProps) {
  if (!selection) {
    return null;
  }
  return (
    <aside className="agent-canvas-detail-panel">
      <header className="agent-canvas-detail-header">
        <div>
          <span>{selection.objectType}</span>
          <h2>{detail?.title || selection.label || "详情"}</h2>
          {detail?.status ? <em>{detail.status}</em> : null}
        </div>
        <button aria-label="关闭详情" onClick={onClose} type="button">
          ×
        </button>
      </header>

      <div className="agent-canvas-detail-body">
        {isLoading ? <DetailLoading /> : null}
        {error ? <DetailError error={error} onRetry={onRetry} /> : null}
        {!isLoading && !error && detail ? (
          <DetailBody detail={detail} onSelectObject={onSelectObject} />
        ) : null}
      </div>
    </aside>
  );
}

function DetailLoading() {
  return (
    <div className="agent-canvas-detail-loading">
      <span />
      <span />
      <span />
    </div>
  );
}

function DetailError({
  error,
  onRetry,
}: {
  error: Error;
  onRetry: () => void;
}) {
  return (
    <section className="agent-canvas-detail-section">
      <h3>加载失败</h3>
      <p>{error.message || "暂时无法加载详情。"}</p>
      <button onClick={onRetry} type="button">
        重试
      </button>
    </section>
  );
}

function DetailBody({
  detail,
  onSelectObject,
}: {
  detail: AgentCanvasDetail;
  onSelectObject: (selection: AgentWorkbenchSelection) => void;
}) {
  if (detail.overview) {
    return (
      <>
        <DetailSection title="创意简报">
          <FieldGrid
            fields={[
              ["标题", detail.overview.brief?.title],
              ["类型", detail.overview.brief?.video_type],
              ["目标受众", detail.overview.brief?.target_audience],
              ["情绪", detail.overview.brief?.tone],
              ["视觉风格", detail.overview.brief?.visual_style],
              ["宽高比", detail.overview.brief?.aspect_ratio],
              ["目标", detail.overview.brief?.objective],
            ]}
          />
          <TextBlock text={detail.overview.brief?.concept} />
        </DetailSection>
        <DetailSection title="项目记忆">
          <FieldGrid
            fields={[
              ["版本", detail.overview.memory?.version],
              ["状态", detail.overview.memory?.status],
              ["核心意图", detail.overview.memory?.core_intent],
              ["Soul", detail.overview.memory?.soul],
            ]}
          />
          <JsonBlock
            label="不可违背约束"
            value={detail.overview.memory?.non_negotiables}
          />
          <JsonBlock
            label="视觉锚点"
            value={detail.overview.memory?.visual_anchors}
          />
        </DetailSection>
        <DetailSection title="关键元素">
          <SummaryList
            items={detail.overview.key_elements.map((item) => ({
              id: item.id,
              title: item.name,
              meta: `${item.type} · ${item.status}`,
            }))}
          />
        </DetailSection>
        <DetailSection title="元素状态">
          <SummaryList
            items={detail.overview.key_element_states.map((item) => ({
              id: item.id,
              title: item.label || item.client_key,
              meta: item.reference_status,
            }))}
          />
        </DetailSection>
      </>
    );
  }

  if (detail.key_element) {
    return (
      <>
        <DetailSection title="关键元素">
          <FieldGrid
            fields={[
              ["名称", detail.key_element.name],
              ["类型", detail.key_element.type],
              ["来源", detail.key_element.source_type],
              ["状态", detail.key_element.status],
            ]}
          />
          <TextBlock text={detail.key_element.description} />
          <JsonBlock label="来源引用" value={detail.key_element.source_refs} />
        </DetailSection>
        <DetailSection title="状态">
          <SummaryList
            items={detail.key_element.states.map((state) => ({
              id: state.id,
              title: state.label,
              meta: state.reference_status,
            }))}
          />
        </DetailSection>
        <DetailSection title="分镜引用">
          <SummaryList
            items={detail.key_element.shot_refs.map((ref) => ({
              id: ref.id,
              title: ref.shot_title || ref.shot_id,
              meta: `${ref.role}${ref.state_label ? ` · ${ref.state_label}` : ""}`,
            }))}
          />
        </DetailSection>
      </>
    );
  }

  if (detail.key_element_state) {
    return (
      <>
        <DetailSection title="元素状态">
          <FieldGrid
            fields={[
              ["标签", detail.key_element_state.label],
              ["引用状态", detail.key_element_state.reference_status],
              ["所属元素", detail.key_element_state.key_element?.name],
              ["默认状态", detail.key_element_state.is_default ? "是" : "否"],
            ]}
          />
          <TextBlock text={detail.key_element_state.visual_description} />
          <TextBlock text={detail.key_element_state.missing_reason} />
          <JsonBlock
            label="状态事实"
            value={detail.key_element_state.state_facts}
          />
        </DetailSection>
        <DetailSection title="依赖分镜">
          <SummaryList
            items={detail.key_element_state.dependent_shots.map((shot) => ({
              id: shot.id,
              title: shot.title || shot.client_key,
              meta: shot.status,
            }))}
          />
        </DetailSection>
      </>
    );
  }

  if (detail.scene) {
    return (
      <>
        <DetailSection title="场景">
          <FieldGrid
            fields={[
              ["标题", detail.scene.title],
              ["位置", detail.scene.location],
              ["情绪", detail.scene.mood],
              ["状态", detail.scene.status],
            ]}
          />
          <TextBlock text={detail.scene.description} />
        </DetailSection>
        <DetailSection title="分镜">
          <SummaryList
            items={detail.scene.shots.map((shot) => ({
              id: shot.id,
              title: `${shot.client_key} · ${shot.title}`,
              meta: shot.status,
            }))}
          />
        </DetailSection>
      </>
    );
  }

  if (detail.shot) {
    return <ShotDetail onSelectObject={onSelectObject} shot={detail.shot} />;
  }

  if (detail.artifact) {
    return <ArtifactDetail artifact={detail.artifact} />;
  }

  if (detail.render_plan) {
    return (
      <>
        <DetailSection title="RenderPlan">
          <FieldGrid
            fields={[
              ["阶段", detail.render_plan.target_phase],
              ["操作", detail.render_plan.operation],
              ["任务类型", detail.render_plan.task_type],
              ["状态", detail.render_plan.status],
              ["版本", detail.render_plan.revision],
              ["Prompt Profile", detail.render_plan.model_prompt_profile],
            ]}
          />
          <TextBlock title="Rationale" text={detail.render_plan.rationale} />
          <JsonBlock
            label="Reference bindings"
            value={detail.render_plan.reference_bindings}
          />
          <JsonBlock
            label="Subject bindings"
            value={detail.render_plan.subject_bindings}
          />
          <JsonBlock
            label="Prompt parts"
            value={detail.render_plan.prompt_parts}
          />
          <JsonBlock label="模型参数" value={detail.render_plan.params} />
          <JsonBlock
            label="Blocker / 错误"
            value={detail.render_plan.blocker}
          />
        </DetailSection>
        <DetailSection title="Compiled Prompt">
          <PromptBlock text={detail.render_plan.compiled_prompt} />
          <JsonBlock
            label="Compiled request"
            value={detail.render_plan.compiled_request}
            collapsed
          />
          <JsonBlock
            label="Prompt audit"
            value={detail.render_plan.prompt_audit}
            collapsed
          />
        </DetailSection>
        <DetailSection title="输出和评审">
          <FieldGrid
            fields={[
              ["输出节点", detail.render_plan.output_node?.title],
              ["输出状态", detail.render_plan.output_node?.status],
              ["输出版本", detail.render_plan.output_version?.version_no],
            ]}
          />
          <IssueList issues={detail.render_plan.issues} />
        </DetailSection>
      </>
    );
  }

  if (detail.review) {
    return (
      <>
        <ReviewRecordDetail review={detail.review.review} />
        <DetailSection title="问题">
          <IssueList issues={detail.review.issues} />
        </DetailSection>
      </>
    );
  }

  if (detail.issue) {
    return (
      <>
        <DetailSection title="问题">
          <FieldGrid
            fields={[
              ["标题", detail.issue.title],
              ["维度", detail.issue.dimension],
              ["严重级别", detail.issue.severity],
              ["状态", detail.issue.status],
              [
                "目标",
                `${detail.issue.target_object_type}:${detail.issue.target_object_id}`,
              ],
              [
                "需要用户确认",
                detail.issue.requires_user_confirmation ? "是" : "否",
              ],
            ]}
          />
          <TextBlock title="描述" text={detail.issue.description} />
          <TextBlock title="证据" text={detail.issue.evidence} />
          <TextBlock title="建议修复" text={detail.issue.suggested_fix} />
          <TextBlock title="修复提示" text={detail.issue.fix_hint} />
        </DetailSection>
        {detail.issue.review ? (
          <ReviewRecordDetail review={detail.issue.review} />
        ) : null}
      </>
    );
  }

  return (
    <DetailSection title="详情">
      <p>没有可展示的详情。</p>
    </DetailSection>
  );
}

function ArtifactDetail({ artifact }: { artifact: AgentCanvasArtifactDetail }) {
  const previewUrl = artifactPreviewUrl(artifact);
  return (
    <>
      <DetailSection title="媒体预览">
        <div className="agent-canvas-artifact-preview">
          {previewUrl ? (
            artifact.node.node_type === "video" ? (
              <video
                controls
                poster={artifact.current_version?.asset?.access_url}
                src={previewUrl}
              />
            ) : (
              <img alt={artifact.node.title || "媒体预览"} src={previewUrl} />
            )
          ) : (
            <div>暂无可预览媒体</div>
          )}
        </div>
      </DetailSection>
      <DetailSection title="媒体节点">
        <FieldGrid
          fields={[
            ["标题", artifact.node.title],
            ["类型", artifact.node.node_type],
            ["状态", artifact.node.status],
            ["操作", artifact.node.operation_type],
            [
              "模型",
              [artifact.node.model_provider, artifact.node.model_id]
                .filter(Boolean)
                .join(" / "),
            ],
            ["当前版本", artifact.node.current_version_id],
          ]}
        />
        <TextBlock title="Prompt" text={artifact.node.prompt} />
        <JsonBlock label="模型参数" value={artifact.node.model_params} />
        <JsonBlock label="Metadata" value={artifact.node.metadata} collapsed />
      </DetailSection>
      <DetailSection title="版本">
        <SummaryList
          items={artifact.versions.map((version) => ({
            id: version.id,
            title: `Version ${version.version_no}${version.winner ? " · winner" : ""}`,
            meta: version.error_message || version.status,
          }))}
        />
      </DetailSection>
      <DetailSection title="生成任务">
        <SummaryList
          items={artifact.generation_jobs.map((job) => ({
            id: job.id,
            title: `${job.provider} / ${job.model_id}`,
            meta: job.error_message || `${job.status} · ${job.progress}%`,
          }))}
        />
      </DetailSection>
      <DetailSection title="生成记录">
        <SummaryList
          items={artifact.render_plans.map((plan) => ({
            id: plan.id,
            title: `${plan.target_phase} / ${plan.operation}`,
            meta: `${plan.status} · r${plan.revision}`,
          }))}
        />
        <SummaryList
          items={artifact.reviews.map((review) => ({
            id: review.id,
            title: `${review.target_phase} review`,
            meta: [
              review.status,
              review.overall_score ? `score ${review.overall_score}` : "",
            ]
              .filter(Boolean)
              .join(" · "),
          }))}
        />
        <IssueList issues={artifact.issues} />
      </DetailSection>
    </>
  );
}

function ShotDetail({
  onSelectObject,
  shot,
}: {
  onSelectObject: (selection: AgentWorkbenchSelection) => void;
  shot: AgentCanvasShotDetail;
}) {
  return (
    <>
      <DetailSection title="分镜脚本">
        <FieldGrid
          fields={[
            ["编号", shot.client_key],
            ["状态", shot.status],
            ["类型", shot.shot_kind],
            ["时长", shot.duration_sec],
            ["叙事目的", shot.narrative_purpose],
          ]}
        />
        <TextBlock title="创意" text={shot.creative_text} />
        <TextBlock title="视觉意图" text={shot.visual_intent} />
        <TextBlock title="动作" text={shot.action_text} />
        <TextBlock title="镜头" text={shot.camera_intent} />
        <TextBlock title="旁白" text={shot.narration} />
        <JsonBlock label="音频计划" value={shot.audio_plan} />
      </DetailSection>

      <DetailSection title="引用依赖">
        <DetailSubsection title="关键元素">
          <SummaryList
            items={shot.key_elements.map((ref) => ({
              id: ref.id,
              title: ref.key_element_name || ref.key_element_id,
              meta: `${ref.role}${ref.state_label ? ` · ${ref.state_label}` : ""}`,
            }))}
          />
        </DetailSubsection>
        <DetailSubsection title="分镜依赖">
          <SummaryList
            items={shot.dependencies.map((dep) => ({
              id: dep.id,
              title: dep.dependency_type,
              meta:
                dep.reason ||
                [dep.required_artifact, dep.injection_role, dep.blocking_phase]
                  .filter(Boolean)
                  .join(" · ") ||
                `${dep.from_shot_id} → ${dep.to_shot_id}`,
            }))}
          />
        </DetailSubsection>
      </DetailSection>

      <DetailSection title="生成产物">
        <OutputCardGrid
          artifacts={shot.artifacts}
          issues={shot.issues}
          onSelectObject={onSelectObject}
          renderPlanCount={shot.render_plans.length}
          reviews={shot.reviews}
        />
      </DetailSection>
    </>
  );
}

function OutputCardGrid({
  artifacts,
  issues,
  onSelectObject,
  renderPlanCount,
  reviews,
}: {
  artifacts: AgentCanvasArtifactSlot[];
  issues: AgentCanvasIssueSummary[];
  onSelectObject: (selection: AgentWorkbenchSelection) => void;
  renderPlanCount: number;
  reviews: { id: string; status: string; verdict: string; score?: number }[];
}) {
  return (
    <>
      {artifacts.length > 0 ? (
        <div className="agent-canvas-output-grid">
          {artifacts.map((artifact, index) => (
            <OutputCard
              artifact={artifact}
              index={index}
              key={artifact.node_id || `${artifact.kind}-${index}`}
              onSelectObject={onSelectObject}
            />
          ))}
        </div>
      ) : (
        <p className="agent-canvas-detail-muted">暂无生成产物</p>
      )}
      <div className="agent-canvas-output-metrics">
        <span>{artifacts.length} 个产物</span>
        <span>{renderPlanCount} 个生成计划</span>
        <span>{reviews.length} 个评审</span>
        <span>{issues.length} 个问题</span>
      </div>
      <IssueList issues={issues} />
    </>
  );
}

function OutputCard({
  artifact,
  index,
  onSelectObject,
}: {
  artifact: AgentCanvasArtifactSlot;
  index: number;
  onSelectObject: (selection: AgentWorkbenchSelection) => void;
}) {
  const previewUrl = artifact.thumbnail_url || artifact.access_url;
  const title = artifact.title || outputKindLabel(artifact.kind, index);
  const selectable = Boolean(artifact.node_id);
  return (
    <button
      className="agent-canvas-output-card"
      data-kind={artifact.kind}
      data-status={artifact.status}
      disabled={!selectable}
      onClick={() => {
        if (!artifact.node_id) {
          return;
        }
        onSelectObject({
          objectType: "artifact",
          objectId: artifact.node_id,
          label: title,
        });
      }}
      type="button"
    >
      <div className="agent-canvas-output-card-preview">
        {previewUrl ? (
          artifact.kind === "shot_video" ? (
            <video
              muted
              playsInline
              poster={artifact.thumbnail_url}
              src={artifact.access_url || artifact.thumbnail_url}
            />
          ) : (
            <img alt={title} src={previewUrl} />
          )
        ) : (
          <span>{artifactPlaceholderText(artifact.status)}</span>
        )}
      </div>
      <div className="agent-canvas-output-card-copy">
        <strong>{title}</strong>
        <span>{artifact.error_message || artifact.status}</span>
      </div>
    </button>
  );
}

function DetailSubsection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <div className="agent-canvas-detail-subsection">
      <h4>{title}</h4>
      {children}
    </div>
  );
}

function outputKindLabel(kind: string, index: number) {
  if (kind === "shot_video") {
    return "分镜视频";
  }
  if (kind === "preview_image") {
    return `预览图 ${index + 1}`;
  }
  return kind || `产物 ${index + 1}`;
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
  return "暂无预览";
}

function artifactPreviewUrl(artifact: AgentCanvasArtifactDetail) {
  return (
    artifact.current_version?.asset?.access_url ||
    artifact.asset?.access_url ||
    artifact.asset?.storage_url ||
    ""
  );
}

function ReviewRecordDetail({ review }: { review: AgentCanvasReviewRecord }) {
  return (
    <DetailSection title="Reviewer">
      <FieldGrid
        fields={[
          ["任务", review.target_phase],
          ["状态", review.status],
          ["总分", review.overall_score],
          ["尝试", `${review.attempt_no}/${review.max_attempts}`],
          [
            "模型",
            [review.model_provider, review.model_id]
              .filter(Boolean)
              .join(" / "),
          ],
        ]}
      />
      <RubricGrid rubric={review.rubric} />
      <TextBlock title="Critique" text={review.critique} />
      <JsonBlock
        label="Retry recommendation"
        value={review.retry_recommendation}
      />
    </DetailSection>
  );
}

function DetailSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section className="agent-canvas-detail-section">
      <h3>{title}</h3>
      {children}
    </section>
  );
}

function FieldGrid({
  fields,
}: {
  fields: [string, string | number | boolean | null | undefined][];
}) {
  const visible = fields.filter(
    ([, value]) => value !== undefined && value !== "",
  );
  if (visible.length === 0) {
    return <p className="agent-canvas-detail-muted">未填写</p>;
  }
  return (
    <dl className="agent-canvas-detail-fields">
      {visible.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value === null ? "未填写" : String(value)}</dd>
        </div>
      ))}
    </dl>
  );
}

function TextBlock({ title, text }: { title?: string; text?: string }) {
  if (!text) {
    return null;
  }
  return (
    <div className="agent-canvas-detail-text-block">
      {title ? <h4>{title}</h4> : null}
      <p>{text}</p>
    </div>
  );
}

function PromptBlock({ text }: { text?: string }) {
  if (!text) {
    return <p className="agent-canvas-detail-muted">未编译</p>;
  }
  return <pre className="agent-canvas-detail-prompt">{text}</pre>;
}

function JsonBlock({
  collapsed,
  label,
  value,
}: {
  collapsed?: boolean;
  label: string;
  value?: AgentJsonValue;
}) {
  if (value === undefined || value === null) {
    return null;
  }
  const body = JSON.stringify(value, null, 2);
  if (collapsed) {
    return (
      <details className="agent-canvas-detail-json">
        <summary>{label}</summary>
        <pre>{body}</pre>
      </details>
    );
  }
  return (
    <div className="agent-canvas-detail-json">
      <h4>{label}</h4>
      <pre>{body}</pre>
    </div>
  );
}

function RubricGrid({ rubric }: { rubric: Record<string, AgentJsonValue> }) {
  const entries = Object.entries(rubric);
  if (entries.length === 0) {
    return <p className="agent-canvas-detail-muted">暂无 rubric</p>;
  }
  return (
    <div className="agent-canvas-detail-rubric">
      {entries.map(([axis, value]) => (
        <div key={axis}>
          <strong>{axis}</strong>
          <span>{rubricValueLabel(value)}</span>
        </div>
      ))}
    </div>
  );
}

function rubricValueLabel(value: AgentJsonValue) {
  if (
    value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    "score" in value
  ) {
    const score = value.score;
    if (typeof score === "number" || typeof score === "string") {
      return String(score);
    }
  }
  if (typeof value === "string" || typeof value === "number") {
    return String(value);
  }
  return JSON.stringify(value);
}

function IssueList({ issues }: { issues: AgentCanvasIssueSummary[] }) {
  if (issues.length === 0) {
    return <p className="agent-canvas-detail-muted">无开放问题</p>;
  }
  return (
    <ul className="agent-canvas-detail-issues">
      {issues.map((issue) => (
        <li key={issue.id}>
          <strong>{issue.title}</strong>
          <span>
            {issue.severity} · {issue.dimension}
          </span>
          {issue.suggested_fix ? <p>{issue.suggested_fix}</p> : null}
        </li>
      ))}
    </ul>
  );
}

function SummaryList({
  items,
}: {
  items: { id: string; title: string; meta?: string }[];
}) {
  if (items.length === 0) {
    return <p className="agent-canvas-detail-muted">暂无</p>;
  }
  return (
    <ul className="agent-canvas-detail-summary-list">
      {items.map((item) => (
        <li key={item.id}>
          <strong>{item.title}</strong>
          {item.meta ? <span>{item.meta}</span> : null}
        </li>
      ))}
    </ul>
  );
}
