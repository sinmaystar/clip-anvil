import { useCallback, useEffect, useRef, useState } from "react";
import type {
  ArtifactVersion,
  MediaEdge,
  MediaGroup,
  MediaNode,
  ModelCapability,
  NodeProductionState,
  ReferencePackItem,
  updateMediaNode,
} from "../lib/api";
import {
  getEdgeDetail,
  getGroupMembers,
} from "../lib/canvasSelectors";
import {
  capabilitiesForNode,
  capabilityKey,
  defaultOperationForNode,
  modelParamsForCapability,
  productionConfigPatchForOperation,
  productionConfigPatchForRun,
  productionConfigPatchForSelectedModel,
  retryDisabledReason,
  runDisabledReason,
  selectedCapabilityKey,
  simplifiedOperationOptionsForNode,
} from "../lib/productionPanel";
import {
  staleReasonText,
  versionCallRecordBlocks,
  versionDetailRows,
  versionHasCallRecord,
  versionRows,
} from "../lib/productionPreview";
import {
  isManualTextMaterialNode,
  isSourceMaterialNode,
  materialKindLabel,
} from "../lib/sourceMaterial";
import {
  candidateReferencePackMembers,
  memberIdsAfterToggle,
  memberNodesForPack,
} from "../lib/referencePack";
import {
  buildPromptRefDocument,
  candidatePromptRefNodes,
  collectPromptRefMentions,
  inputReferenceState,
} from "../lib/promptRefs";
import { MarkdownPreview } from "./MarkdownPreview";

type NodePatch = Parameters<typeof updateMediaNode>[1];

interface PropertyPanelProps {
  edges: MediaEdge[];
  groups: MediaGroup[];
  nodes: MediaNode[];
  selectedEdgeId: string | null;
  selectedGroupId: string | null;
  selectedNodeId: string | null;
  isModelCapabilitiesLoading: boolean;
  isProductionStateLoading: boolean;
  isReferencePackItemsLoading: boolean;
  isRetryingJob: boolean;
  isRunningNode: boolean;
  isSelectingVersion: boolean;
  isUpdatingReferencePackItems: boolean;
  isUpdatingGroupMembers: boolean;
  isUpdatingNode: boolean;
  assetReviewRequestKey?: number;
  modelCapabilities: ModelCapability[];
  nodeProductionState: NodeProductionState | null;
  referencePackItems: ReferencePackItem[];
  onAddGroupMember?: (groupId: string, nodeId: string) => void;
  onDeleteEdge?: (edgeId: string) => void;
  onDeleteGroup?: (groupId: string) => void;
  onRemoveGroupMember?: (groupId: string, nodeId: string) => void;
  onRenameGroup?: (groupId: string, name: string) => void;
  onReplaceReferencePackItems: (
    packNodeId: string,
    memberNodeIds: string[],
  ) => void;
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onRetryJob: (jobId: string) => void;
  onRunNode: (nodeId: string, patch?: NodePatch) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}

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

export function PropertyPanel({
  edges,
  groups,
  nodes,
  selectedEdgeId,
  selectedGroupId,
  selectedNodeId,
  isModelCapabilitiesLoading,
  isProductionStateLoading,
  isReferencePackItemsLoading,
  isRetryingJob,
  isRunningNode,
  isSelectingVersion,
  isUpdatingReferencePackItems,
  isUpdatingGroupMembers,
  isUpdatingNode,
  assetReviewRequestKey = 0,
  modelCapabilities,
  nodeProductionState,
  referencePackItems,
  onAddGroupMember,
  onDeleteEdge,
  onDeleteGroup,
  onRemoveGroupMember,
  onRenameGroup,
  onReplaceReferencePackItems,
  onPromptRefSelect,
  onRetryJob,
  onRunNode,
  onSelectVersion,
  onUpdateNode,
}: PropertyPanelProps) {
  const selectedEdge = getEdgeDetail(nodes, edges, selectedEdgeId);
  const selectedGroup =
    groups.find((group) => group.id === selectedGroupId) ?? null;
  const selectedNode = nodes.find((node) => node.id === selectedNodeId) ?? null;

  if (selectedEdge) {
    return (
      <EdgePropertyPanel detail={selectedEdge} onDeleteEdge={onDeleteEdge} />
    );
  }

  if (selectedGroup) {
    return (
      <GroupPropertyPanel
        candidateNodes={getGroupCandidateNodes(nodes, selectedGroup)}
        group={selectedGroup}
        isUpdatingMembers={isUpdatingGroupMembers}
        memberNodes={getGroupMembers(nodes, selectedGroup)}
        onAddGroupMember={onAddGroupMember}
        onDeleteGroup={onDeleteGroup}
        onRemoveGroupMember={onRemoveGroupMember}
        onRenameGroup={onRenameGroup}
      />
    );
  }

  if (selectedNode) {
    if (isSourceMaterialNode(selectedNode)) {
      return (
        <SourceMaterialPanel
          isUpdatingNode={isUpdatingNode}
          node={selectedNode}
          onUpdateNode={onUpdateNode}
        />
      );
    }
    return (
      <NodePropertyPanel
        edges={edges}
        isModelCapabilitiesLoading={isModelCapabilitiesLoading}
        isProductionStateLoading={isProductionStateLoading}
        isReferencePackItemsLoading={isReferencePackItemsLoading}
        isRetryingJob={isRetryingJob}
        isRunningNode={isRunningNode}
        isSelectingVersion={isSelectingVersion}
        isUpdatingReferencePackItems={isUpdatingReferencePackItems}
        isUpdatingNode={isUpdatingNode}
        assetReviewRequestKey={assetReviewRequestKey}
        modelCapabilities={modelCapabilities}
        node={selectedNode}
        nodeProductionState={nodeProductionState}
        nodes={nodes}
        referencePackItems={referencePackItems}
        onReplaceReferencePackItems={onReplaceReferencePackItems}
        onPromptRefSelect={onPromptRefSelect}
        onRetryJob={onRetryJob}
        onRunNode={onRunNode}
        onSelectVersion={onSelectVersion}
        onUpdateNode={onUpdateNode}
      />
    );
  }

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Inspector" title="未选择" />
      <p className="property-empty">选择节点或分组查看属性。</p>
    </aside>
  );
}

function GroupPropertyPanel({
  candidateNodes,
  group,
  isUpdatingMembers,
  memberNodes,
  onAddGroupMember,
  onDeleteGroup,
  onRemoveGroupMember,
  onRenameGroup,
}: {
  candidateNodes: MediaNode[];
  group: MediaGroup;
  isUpdatingMembers: boolean;
  memberNodes: MediaNode[];
  onAddGroupMember?: (groupId: string, nodeId: string) => void;
  onDeleteGroup?: (groupId: string) => void;
  onRemoveGroupMember?: (groupId: string, nodeId: string) => void;
  onRenameGroup?: (groupId: string, name: string) => void;
}) {
  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Group" title={group.name} />
      {onRenameGroup ? (
        <label className="property-field">
          <span>名称</span>
          <input
            defaultValue={group.name}
            key={group.id}
            onBlur={(event) => {
              const nextName = event.currentTarget.value.trim();
              if (nextName && nextName !== group.name) {
                onRenameGroup(group.id, nextName);
              }
            }}
          />
        </label>
      ) : null}
      <dl className="property-list">
        <div>
          <dt>成员数量</dt>
          <dd>{memberNodes.length}</dd>
        </div>
        <div>
          <dt>排序</dt>
          <dd>{group.sort_order}</dd>
        </div>
      </dl>
      {onAddGroupMember ? (
        <label className="property-field">
          <span>添加成员</span>
          <select
            disabled={isUpdatingMembers || candidateNodes.length === 0}
            onChange={(event) => {
              const nodeId = event.currentTarget.value;
              if (nodeId) {
                onAddGroupMember(group.id, nodeId);
              }
            }}
            value=""
          >
            <option value="">
              {isUpdatingMembers
                ? "更新中"
                : candidateNodes.length > 0
                  ? "选择节点"
                  : "没有可添加节点"}
            </option>
            {candidateNodes.map((node) => (
              <option key={node.id} value={node.id}>
                {node.title}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      <div className="property-section">
        <p className="studio-section-label">Members</p>
        {memberNodes.length > 0 ? (
          memberNodes.map((node) => (
            <div className="property-row property-row-action" key={node.id}>
              <span>{node.title}</span>
              {onRemoveGroupMember ? (
                <button
                  onClick={() => onRemoveGroupMember(group.id, node.id)}
                  type="button"
                >
                  移出
                </button>
              ) : null}
            </div>
          ))
        ) : (
          <p className="property-empty">这个分组暂时没有成员。</p>
        )}
      </div>
      {onDeleteGroup ? (
        <button
          className="studio-secondary-button property-danger"
          onClick={() => onDeleteGroup(group.id)}
          type="button"
        >
          删除分组
        </button>
      ) : null}
    </aside>
  );
}

function getGroupCandidateNodes(nodes: MediaNode[], group: MediaGroup) {
  const memberIds = new Set(group.node_ids ?? []);
  return nodes.filter((node) => !memberIds.has(node.id));
}

function SourceMaterialPanel({
  isUpdatingNode,
  node,
  onUpdateNode,
}: {
  isUpdatingNode: boolean;
  node: MediaNode;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const [titleValue, setTitleValue] = useState(node.title);
  const [isTitleEditing, setIsTitleEditing] = useState(false);
  const [contentValue, setContentValue] = useState(node.prompt);
  const isManualTextMaterial = isManualTextMaterialNode(node);
  const titleValueRef = useRef(titleValue);
  const nodeTitleRef = useRef(node.title);
  const contentValueRef = useRef(contentValue);
  const nodePromptRef = useRef(node.prompt);
  const onUpdateNodeRef = useRef(onUpdateNode);

  useEffect(() => {
    onUpdateNodeRef.current = onUpdateNode;
  }, [onUpdateNode]);

  useEffect(() => {
    setTitleValue(node.title);
    titleValueRef.current = node.title;
    nodeTitleRef.current = node.title;
    setContentValue(node.prompt);
    contentValueRef.current = node.prompt;
    nodePromptRef.current = node.prompt;
    setIsTitleEditing(false);
  }, [node.id, node.title, node.prompt]);

  const commitTitle = useCallback(() => {
    const title = titleValueRef.current.trim();
    if (title && title !== nodeTitleRef.current) {
      onUpdateNodeRef.current(node.id, { title });
      nodeTitleRef.current = title;
    }
    setTitleValue(title || nodeTitleRef.current);
    setIsTitleEditing(false);
  }, [node.id]);

  const cancelTitleEdit = useCallback(() => {
    titleValueRef.current = nodeTitleRef.current;
    setTitleValue(nodeTitleRef.current);
    setIsTitleEditing(false);
  }, []);

  const commitContent = useCallback(() => {
    const content = contentValueRef.current;
    if (content === nodePromptRef.current) {
      return;
    }
    onUpdateNodeRef.current(node.id, {
      prompt: content,
      prompt_rich: {
        version: 1,
        source: "textarea-at",
        text: content,
      },
    });
    nodePromptRef.current = content;
  }, [node.id]);

  useEffect(() => {
    return () => {
      commitTitle();
      if (isManualTextMaterial) {
        commitContent();
      }
    };
  }, [commitContent, commitTitle, isManualTextMaterial]);

  const previewVersion = buildSourceMaterialPreviewVersion(node);

  return (
    <aside className="property-panel node-production-panel">
      <div className="property-panel-title-row">
        <EditablePanelHeader
          eyebrow={materialKindLabel(node)}
          isEditing={isTitleEditing}
          isUpdating={isUpdatingNode}
          onCancel={cancelTitleEdit}
          onCommit={commitTitle}
          onEdit={() => setIsTitleEditing(true)}
          onTitleChange={(next) => {
            titleValueRef.current = next;
            setTitleValue(next);
          }}
          title={titleValue}
        />
        <span className="property-status-pill" data-status={node.status}>
          可用
        </span>
      </div>
      {isManualTextMaterial ? (
        <label className="property-field property-content-field">
          <span>内容</span>
          <textarea
            disabled={isUpdatingNode}
            onBlur={commitContent}
            onChange={(event) => {
              const next = event.currentTarget.value;
              contentValueRef.current = next;
              setContentValue(next);
            }}
            placeholder="粘贴视频脚本、商品卖点或参考文案"
            rows={12}
            value={contentValue}
          />
        </label>
      ) : (
        <div className="property-section property-source-preview">
          <p className="studio-section-label">素材预览</p>
          <VersionPreviewBody version={previewVersion} />
        </div>
      )}
      <p className="property-empty">
        这是用户素材节点，可作为依赖输入或加入参考包，不需要运行模型。
      </p>
    </aside>
  );
}

function buildSourceMaterialPreviewVersion(node: MediaNode): ArtifactVersion {
  const accessUrl = node.asset_url ?? node.production_preview?.access_url;
  const assetType = node.node_type === "reference_pack" ? "json" : node.node_type;
  return {
    id: node.asset_id ?? node.id,
    workspace_id: node.workspace_id,
    node_id: node.id,
    asset_id: node.asset_id ?? undefined,
    version_no: 1,
    winner: true,
    output: {},
    input_hash: "",
    status: node.status === "failed" ? "failed" : "succeeded",
    progress: 100,
    provider_request: {},
    provider_response: {},
    asset: {
      id: node.asset_id ?? node.id,
      type: assetType,
      mime: node.production_preview?.mime ?? "",
      access_url: accessUrl,
      text_content: node.node_type === "text" ? node.prompt : undefined,
      metadata: {},
    },
    created_at: node.created_at,
  };
}

function NodePropertyPanel({
  edges,
  isModelCapabilitiesLoading,
  isProductionStateLoading,
  isReferencePackItemsLoading,
  isRetryingJob,
  isRunningNode,
  isSelectingVersion,
  isUpdatingReferencePackItems,
  isUpdatingNode,
  assetReviewRequestKey,
  modelCapabilities,
  node,
  nodeProductionState,
  nodes,
  referencePackItems,
  onReplaceReferencePackItems,
  onPromptRefSelect,
  onRetryJob,
  onRunNode,
  onSelectVersion,
  onUpdateNode,
}: {
  edges: MediaEdge[];
  isModelCapabilitiesLoading: boolean;
  isProductionStateLoading: boolean;
  isReferencePackItemsLoading: boolean;
  isRetryingJob: boolean;
  isRunningNode: boolean;
  isSelectingVersion: boolean;
  isUpdatingReferencePackItems: boolean;
  isUpdatingNode: boolean;
  assetReviewRequestKey: number;
  modelCapabilities: ModelCapability[];
  node: MediaNode;
  nodeProductionState: NodeProductionState | null;
  nodes: MediaNode[];
  referencePackItems: ReferencePackItem[];
  onReplaceReferencePackItems: (
    packNodeId: string,
    memberNodeIds: string[],
  ) => void;
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onRetryJob: (jobId: string) => void;
  onRunNode: (nodeId: string, patch?: NodePatch) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const refState = inputReferenceState(node, nodes, edges);
  const isReferencePack = node.node_type === "reference_pack";
  const operation = defaultOperationForNode(node);
  const operationOptions = simplifiedOperationOptionsForNode(node);
  const compatibleCapabilities = capabilitiesForNode(
    node,
    modelCapabilities,
    operation,
  );
  const selectedModelKey = selectedCapabilityKey(node, modelCapabilities);
  const selectedCapability = compatibleCapabilities.find(
    (capability) => capabilityKey(capability) === selectedModelKey,
  );
  const modelParams = modelParamsForCapability(node, selectedCapability);
  const disabledReason = runDisabledReason(
    node,
    nodeProductionState,
    modelCapabilities,
    refState,
  );
  const latestJob = nodeProductionState?.latest_job ?? null;
  const versions = nodeProductionState?.versions ?? [];
  const currentVersion =
    nodeProductionState?.current_version ??
    versions.find((version) => version.winner) ??
    null;
  const retryReason = retryDisabledReason(latestJob);
  const durations = durationOptions(selectedCapability);
  const [titleValue, setTitleValue] = useState(node.title);
  const [previewVersionId, setPreviewVersionId] = useState<string | null>(null);
  const [isTitleEditing, setIsTitleEditing] = useState(false);
  const [detailVersionId, setDetailVersionId] = useState<string | null>(null);
  const [reviewVersionId, setReviewVersionId] = useState<string | null>(null);
  const titleValueRef = useRef(titleValue);
  const nodeTitleRef = useRef(node.title);
  const onUpdateNodeRef = useRef(onUpdateNode);
  const previewVersion =
    (previewVersionId
      ? versions.find((version) => version.id === previewVersionId)
      : null) ??
    currentVersion ??
    versions[0] ??
    null;

  useEffect(() => {
    onUpdateNodeRef.current = onUpdateNode;
  }, [onUpdateNode]);

  useEffect(() => {
    setTitleValue(node.title);
    titleValueRef.current = node.title;
    nodeTitleRef.current = node.title;
    setPreviewVersionId(null);
    setDetailVersionId(null);
    setReviewVersionId(null);
    setIsTitleEditing(false);
  }, [node.id, node.title]);

  useEffect(() => {
    if (
      previewVersionId &&
      !versions.some((version) => version.id === previewVersionId)
    ) {
      setPreviewVersionId(null);
    }
  }, [previewVersionId, versions]);

  const commitTitle = useCallback(() => {
    const title = titleValueRef.current.trim();
    if (title && title !== nodeTitleRef.current) {
      onUpdateNodeRef.current(node.id, { title });
      nodeTitleRef.current = title;
    }
    setTitleValue(title || nodeTitleRef.current);
    setIsTitleEditing(false);
  }, [node.id]);

  const cancelTitleEdit = useCallback(() => {
    titleValueRef.current = nodeTitleRef.current;
    setTitleValue(nodeTitleRef.current);
    setIsTitleEditing(false);
  }, []);

  useEffect(() => {
    return () => {
      commitTitle();
    };
  }, [commitTitle]);

  useEffect(() => {
    if (!assetReviewRequestKey || !previewVersion) {
      return;
    }
    setReviewVersionId(previewVersion.id);
  }, [assetReviewRequestKey, previewVersion]);

  const detailVersion =
    (detailVersionId
      ? versions.find((version) => version.id === detailVersionId)
      : null) ?? null;
  const reviewVersion =
    (reviewVersionId
      ? versions.find((version) => version.id === reviewVersionId)
      : null) ?? null;

  return (
    <aside className="property-panel node-production-panel">
      <div className="property-panel-title-row">
        <EditablePanelHeader
          eyebrow={nodeTypeLabel[node.node_type]}
          isEditing={isTitleEditing}
          isUpdating={isUpdatingNode}
          onCancel={cancelTitleEdit}
          onCommit={commitTitle}
          onEdit={() => setIsTitleEditing(true)}
          onTitleChange={(next) => {
            titleValueRef.current = next;
            setTitleValue(next);
          }}
          title={titleValue}
        />
        <span className="property-status-pill" data-status={node.status}>
          {statusLabel[node.status]}
        </span>
      </div>
      {isReferencePack ? (
        <>
          <div className="property-section">
            <p className="studio-section-label">Reference Pack</p>
            <p className="property-empty">
              Reference Pack 管理已有节点的直接成员；成员关系不会改变分组，也不会自动包含成员的上游依赖。
            </p>
          </div>
          <ReferencePackMembersSection
            candidates={candidateReferencePackMembers(
              node,
              nodes,
              referencePackItems,
            )}
            isLoading={isReferencePackItemsLoading}
            isUpdating={isUpdatingReferencePackItems}
            members={memberNodesForPack(referencePackItems, nodes)}
            onToggleMember={(memberNodeId, checked) =>
              onReplaceReferencePackItems(
                node.id,
                memberIdsAfterToggle(
                  referencePackItems,
                  memberNodeId,
                  checked,
                ),
              )
            }
          />
        </>
      ) : (
        <>
          <div className="property-section property-run-section">
            {disabledReason ? (
              <p className="property-empty">{disabledReason}</p>
            ) : null}
            <button
              className="studio-secondary-button property-run-button"
              disabled={
                Boolean(disabledReason) || isRunningNode || isUpdatingNode
              }
              onClick={() =>
                onRunNode(
                  node.id,
                  productionConfigPatchForRun(node, modelCapabilities) ??
                    undefined,
                )
              }
              type="button"
            >
              {isRunningNode ? "运行中" : "运行节点"}
            </button>
            {latestJob?.status === "failed" ? (
              <>
                {retryReason ? (
                  <p className="property-empty">{retryReason}</p>
                ) : null}
                <button
                  className="studio-secondary-button"
                  disabled={isRetryingJob || Boolean(retryReason)}
                  onClick={() => onRetryJob(latestJob.id)}
                  type="button"
                >
                  {isRetryingJob ? "重试中" : "重试失败任务"}
                </button>
              </>
            ) : null}
          </div>
          <PropertyPromptEditor
            edges={edges}
            isUpdatingNode={isUpdatingNode}
            node={node}
            nodes={nodes}
            onPromptRefSelect={onPromptRefSelect}
            onUpdateNode={onUpdateNode}
          />
          <label className="property-field">
            <span>Operation</span>
            <select
              disabled={isUpdatingNode}
              onChange={(event) =>
                onUpdateNode(
                  node.id,
                  productionConfigPatchForOperation(
                    node,
                    modelCapabilities,
                    event.currentTarget.value,
                  ),
                )
              }
              value={operation}
            >
              {operationOptions.map(({ value, label }) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label className="property-field">
            <span>Model</span>
            <select
              disabled={
                isModelCapabilitiesLoading ||
                isUpdatingNode ||
                compatibleCapabilities.length === 0
              }
              onChange={(event) => {
                onUpdateNode(
                  node.id,
                  productionConfigPatchForSelectedModel(
                    node,
                    modelCapabilities,
                    event.currentTarget.value,
                  ),
                );
              }}
              value={selectedModelKey}
            >
              {compatibleCapabilities.length > 0 ? (
                compatibleCapabilities.map((capability) => (
                  <option
                    key={capabilityKey(capability)}
                    value={capabilityKey(capability)}
                  >
                    {capability.display_name}
                  </option>
                ))
              ) : (
                <option value="">没有兼容模型</option>
              )}
            </select>
          </label>
          <div className="property-section">
            <p className="studio-section-label">Params</p>
            {"temperature" in modelParams ? (
              <label className="property-field">
                <span>Temperature</span>
                <input
                  defaultValue={String(modelParams.temperature)}
                  disabled={isUpdatingNode}
                  max="1"
                  min="0"
                  onBlur={(event) =>
                    onUpdateNode(node.id, {
                      model_params: {
                        ...modelParams,
                        temperature: Number(event.currentTarget.value),
                      },
                    })
                  }
                  step="0.1"
                  type="number"
                />
              </label>
            ) : null}
            {"duration_sec" in modelParams ? (
              <label className="property-field">
                <span>Duration</span>
                <select
                  disabled={isUpdatingNode}
                  onChange={(event) =>
                    onUpdateNode(node.id, {
                      model_params: {
                        ...modelParams,
                        duration_sec: Number(event.currentTarget.value),
                      },
                    })
                  }
                  value={String(modelParams.duration_sec)}
                >
                  {durations.map((duration) => (
                    <option key={duration} value={duration}>
                      {duration}s
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            <label className="property-check-row">
              <input
                checked={Boolean(modelParams.mock_fail)}
                disabled={isUpdatingNode}
                onChange={(event) =>
                  onUpdateNode(node.id, {
                    model_params: {
                      ...modelParams,
                      mock_fail: event.currentTarget.checked,
                    },
                  })
                }
                type="checkbox"
              />
              <span>Mock failure</span>
            </label>
          </div>
          {node.production_preview?.text ? (
            <div className="property-section property-output-section">
              <p className="studio-section-label">Output</p>
              <MarkdownPreview
                value={node.production_preview.text}
                variant="panel"
              />
            </div>
          ) : null}
        </>
      )}
      {!isReferencePack ? (
        <>
          <div className="property-section property-version-section">
            <div className="property-section-heading">
              <span>Versions</span>
              {previewVersion &&
              currentVersion &&
              previewVersion.id !== currentVersion.id ? (
                <button
                  onClick={() => setPreviewVersionId(null)}
                  type="button"
                >
                  回到当前
                </button>
              ) : null}
            </div>
            {isProductionStateLoading ? (
              <p className="property-empty">正在读取 production state。</p>
            ) : versions.length ? (
              <>
                <VersionPreviewPanel
                  currentVersionId={currentVersion?.id ?? null}
                  detailVersionId={detailVersionId}
                  isSelectingVersion={isSelectingVersion}
                  nodeId={node.id}
                  onOpenDetails={(versionId) =>
                    setDetailVersionId((current) =>
                      current === versionId ? null : versionId,
                    )
                  }
                  onOpenReview={(versionId) => setReviewVersionId(versionId)}
                  onSelectVersion={onSelectVersion}
                  version={previewVersion}
                />
                {detailVersion ? (
                  <VersionDetailPanel version={detailVersion} />
                ) : null}
                <div className="property-version-list">
                  {versionRows(versions).map((version) => (
                    <button
                      className="property-version-row"
                      data-active={previewVersion?.id === version.id}
                      data-status={version.status}
                      key={version.id}
                      onClick={() => setPreviewVersionId(version.id)}
                      type="button"
                    >
                      <div>
                        <strong>{version.versionLabel}</strong>
                        <span>{version.statusLabel}</span>
                      </div>
                      <div>
                        <span>{version.assetLabel}</span>
                        <small>{version.assetDetail}</small>
                      </div>
                      <span>{version.inputHash}</span>
                      {version.progressLabel ? (
                        <span>{version.progressLabel}</span>
                      ) : null}
                    </button>
                  ))}
                </div>
              </>
            ) : (
              <p className="property-empty">尚无 artifact version。</p>
            )}
          </div>
        </>
      ) : null}
      <details className="property-section property-details">
        <summary>Stale Reasons</summary>
        {nodeProductionState?.active_stale_reasons.length ? (
          <ul className="property-stale-list">
            {nodeProductionState.active_stale_reasons.map((reason) => (
              <li key={reason.id}>{staleReasonText(reason)}</li>
            ))}
          </ul>
        ) : (
          <p className="property-empty">当前节点没有 active stale reason。</p>
        )}
      </details>
      {reviewVersion ? (
        <AssetReviewOverlay
          currentVersionId={currentVersion?.id ?? null}
          isSelectingVersion={isSelectingVersion}
          nodeTitle={titleValue || node.title}
          nodeId={node.id}
          onClose={() => setReviewVersionId(null)}
          onOpenDetails={(versionId) => setDetailVersionId(versionId)}
          onSelectVersion={onSelectVersion}
          version={reviewVersion}
        />
      ) : null}
    </aside>
  );
}

function VersionPreviewPanel({
  currentVersionId,
  detailVersionId,
  isSelectingVersion,
  nodeId,
  version,
  onOpenDetails,
  onOpenReview,
  onSelectVersion,
}: {
  currentVersionId: string | null;
  detailVersionId: string | null;
  isSelectingVersion: boolean;
  nodeId: string;
  version: ArtifactVersion | null;
  onOpenDetails: (versionId: string) => void;
  onOpenReview: (versionId: string) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
}) {
  if (!version) {
    return <p className="property-empty">尚无可预览版本。</p>;
  }
  const isCurrent = version.id === currentVersionId;
  const canSelect =
    version.status === "succeeded" && Boolean(version.asset) && !isCurrent;

  return (
    <div className="property-version-preview" data-status={version.status}>
      <div className="property-version-preview-header">
        <div>
          <strong>v{version.version_no}</strong>
          <span>{isCurrent ? "current" : version.status}</span>
        </div>
        <div className="property-version-actions">
          <button onClick={() => onOpenReview(version.id)} type="button">
            全屏查看
          </button>
          {versionHasCallRecord(version) ? (
            <button onClick={() => onOpenDetails(version.id)} type="button">
              {detailVersionId === version.id ? "收起详情" : "详情"}
            </button>
          ) : null}
          {canSelect ? (
            <button
              disabled={isSelectingVersion}
              onClick={() => onSelectVersion(nodeId, version.id)}
              type="button"
            >
              设为当前
            </button>
          ) : null}
        </div>
      </div>
      <VersionPreviewBody version={version} />
    </div>
  );
}

function VersionDetailPanel({ version }: { version: ArtifactVersion }) {
  const blocks = versionCallRecordBlocks(version);
  return (
    <div className="property-version-detail">
      <dl className="property-list">
        {versionDetailRows(version).map((row) => (
          <div key={row.label}>
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        ))}
      </dl>
      {blocks.length ? (
        blocks.map((block) => (
          <JsonBlock
            key={block.label}
            label={block.label}
            value={block.value}
          />
        ))
      ) : (
        <p className="property-empty">该版本没有调用记录详情。</p>
      )}
    </div>
  );
}

function AssetReviewOverlay({
  currentVersionId,
  isSelectingVersion,
  nodeId,
  nodeTitle,
  version,
  onClose,
  onOpenDetails,
  onSelectVersion,
}: {
  currentVersionId: string | null;
  isSelectingVersion: boolean;
  nodeId: string;
  nodeTitle: string;
  version: ArtifactVersion;
  onClose: () => void;
  onOpenDetails: (versionId: string) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
}) {
  const isCurrent = version.id === currentVersionId;
  const canSelect =
    version.status === "succeeded" && Boolean(version.asset) && !isCurrent;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <div
      className="asset-review-overlay"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={`${nodeTitle} v${version.version_no} 全屏查看`}
    >
      <div
        className="asset-review-content"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="asset-review-header">
          <div>
            <p>{nodeTitle}</p>
            <h2>
              v{version.version_no} · {isCurrent ? "current" : version.status}
            </h2>
          </div>
          <div className="asset-review-actions">
            {versionHasCallRecord(version) ? (
              <button onClick={() => onOpenDetails(version.id)} type="button">
                详情
              </button>
            ) : null}
            {canSelect ? (
              <button
                disabled={isSelectingVersion}
                onClick={() => onSelectVersion(nodeId, version.id)}
                type="button"
              >
                设为当前
              </button>
            ) : null}
            <button onClick={onClose} type="button">
              关闭
            </button>
          </div>
        </header>
        <div className="asset-review-body">
          <VersionPreviewBody version={version} />
        </div>
      </div>
    </div>
  );
}

function VersionPreviewBody({ version }: { version: ArtifactVersion }) {
  if (version.status === "queued" || version.status === "running") {
    return (
      <div className="property-version-progress">
        <span>{Math.max(0, Math.min(100, Math.round(version.progress)))}%</span>
        <div>
          <i style={{ width: `${Math.max(2, Math.min(100, version.progress))}%` }} />
        </div>
      </div>
    );
  }
  if (version.status === "failed" || version.status === "cancelled") {
    return (
      <div className="property-version-error">
        {version.error_message || version.error_code || "运行失败"}
      </div>
    );
  }
  if (!version.asset) {
    return <p className="property-empty">该版本没有素材。</p>;
  }
  if (version.asset.type === "text") {
    return (
      <MarkdownPreview
        value={version.asset.text_content || ""}
        variant="panel"
      />
    );
  }
  if (version.asset.type === "image" && version.asset.access_url) {
    return (
      <img
        alt={`v${version.version_no}`}
        className="property-version-media"
        src={version.asset.access_url}
      />
    );
  }
  if (version.asset.type === "video" && version.asset.access_url) {
    return (
      <video
        className="property-version-media"
        controls
        src={version.asset.access_url}
      />
    );
  }
  return <p className="property-empty">{version.asset.mime || "素材已生成"}</p>;
}

function PropertyPromptEditor({
  edges,
  isUpdatingNode,
  node,
  nodes,
  onPromptRefSelect,
  onUpdateNode,
}: {
  edges: MediaEdge[];
  isUpdatingNode: boolean;
  node: MediaNode;
  nodes: MediaNode[];
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const [promptValue, setPromptValue] = useState(node.prompt);
  const [isPromptFocused, setIsPromptFocused] = useState(false);
  const isRefMenuOpen = isPromptFocused && promptValue.endsWith("@");
  const candidates = candidatePromptRefNodes(node, nodes, edges);
  const candidatesRef = useRef(candidates);
  const promptValueRef = useRef(promptValue);
  const nodePromptRef = useRef(node.prompt);
  const onUpdateNodeRef = useRef(onUpdateNode);

  useEffect(() => {
    candidatesRef.current = candidates;
  }, [candidates]);

  useEffect(() => {
    onUpdateNodeRef.current = onUpdateNode;
  }, [onUpdateNode]);

  useEffect(() => {
    setPromptValue(node.prompt);
    promptValueRef.current = node.prompt;
    nodePromptRef.current = node.prompt;
  }, [node.id, node.prompt]);

  useEffect(() => {
    setIsPromptFocused(false);
  }, [node.id]);

  const commitPrompt = useCallback(
    (prompt: string) => {
      if (prompt === nodePromptRef.current) {
        return;
      }
      onUpdateNodeRef.current(node.id, {
        prompt,
        prompt_refs: buildPromptRefDocument(
          collectPromptRefMentions(prompt, candidatesRef.current),
          candidatesRef.current,
        ),
        prompt_rich: {
          version: 1,
          source: "textarea-at",
          text: prompt,
        },
      });
      nodePromptRef.current = prompt;
    },
    [node.id],
  );

  useEffect(() => {
    return () => {
      commitPrompt(promptValueRef.current);
    };
  }, [commitPrompt]);

  return (
    <label className="property-field property-prompt-editor">
      <span>Prompt</span>
      <textarea
        disabled={isUpdatingNode}
        onBlur={(event) => {
          commitPrompt(event.currentTarget.value);
          setIsPromptFocused(false);
        }}
        onChange={(event) => {
          const next = event.currentTarget.value;
          promptValueRef.current = next;
          setPromptValue(next);
        }}
        onFocus={() => setIsPromptFocused(true)}
        placeholder="输入生成文本、画面描述或旁白方向"
        value={promptValue}
      />
      {isRefMenuOpen ? (
        <div className="prompt-ref-menu">
          {candidates.length > 0 ? (
            candidates.map((candidate) => (
              <button
                key={candidate.id}
                onClick={() => {
                  const token = `@${candidate.title || "未命名"}`;
                  const next = promptValue.replace(/@$/, token);
                  promptValueRef.current = next;
                  setPromptValue(next);
                  onPromptRefSelect(node, candidate, next);
                }}
                onMouseDown={(event) => event.preventDefault()}
                type="button"
              >
                <span>{candidate.title}</span>
                <small>{candidate.node_type}</small>
              </button>
            ))
          ) : (
            <span className="prompt-ref-menu-empty">没有可引用节点</span>
          )}
        </div>
      ) : null}
    </label>
  );
}

function EdgePropertyPanel({
  detail,
  onDeleteEdge,
}: {
  detail: NonNullable<ReturnType<typeof getEdgeDetail>>;
  onDeleteEdge?: (edgeId: string) => void;
}) {
  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Dependency" title="依赖连线" />
      <div className="property-section">
        <p className="studio-section-label">方向</p>
        <div className="property-flow">
          <NodeChip node={detail.fromNode} />
          <span aria-hidden="true">→</span>
          <NodeChip node={detail.toNode} />
        </div>
        <p className="property-empty">
          {detail.toNode.title} 依赖 {detail.fromNode.title}。
        </p>
      </div>
      <dl className="property-list">
        <div>
          <dt>类型</dt>
          <dd>dependency</dd>
        </div>
        <div>
          <dt>来源</dt>
          <dd>{detail.edge.source}</dd>
        </div>
      </dl>
      {onDeleteEdge ? (
        <button
          className="studio-secondary-button property-danger"
          onClick={() => onDeleteEdge(detail.edge.id)}
          type="button"
        >
          删除依赖
        </button>
      ) : null}
    </aside>
  );
}

function ReferencePackMembersSection({
  candidates,
  isLoading,
  isUpdating,
  members,
  onToggleMember,
}: {
  candidates: MediaNode[];
  isLoading: boolean;
  isUpdating: boolean;
  members: MediaNode[];
  onToggleMember: (memberNodeId: string, checked: boolean) => void;
}) {
  return (
    <div className="property-section">
      <p className="studio-section-label">Members</p>
      {isLoading ? (
        <p className="property-empty">正在读取成员。</p>
      ) : members.length ? (
        <div className="reference-pack-member-list">
          {members.map((member, index) => (
            <div className="reference-pack-member-row" key={member.id}>
              <span>{index + 1}</span>
              <NodeChip node={member} />
              <button
                className="studio-secondary-button"
                disabled={isUpdating}
                onClick={() => onToggleMember(member.id, false)}
                type="button"
              >
                移除
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="property-empty">暂无成员。</p>
      )}
      <p className="studio-section-label">Candidates</p>
      {candidates.length ? (
        <div className="reference-pack-candidate-list">
          {candidates.map((candidate) => (
            <button
              className="reference-pack-candidate-row"
              disabled={isUpdating}
              key={candidate.id}
              onClick={() => onToggleMember(candidate.id, true)}
              type="button"
            >
              <NodeChip node={candidate} />
              <span>添加</span>
            </button>
          ))}
        </div>
      ) : (
        <p className="property-empty">没有可添加的非 Pack 节点。</p>
      )}
    </div>
  );
}

function NodeChip({ node }: { node: MediaNode }) {
  return (
    <span className="property-node-chip">
      <span>{node.title}</span>
      <small>{nodeTypeLabel[node.node_type]}</small>
    </span>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="property-json-block">
      <span>{label}</span>
      <pre>{formatInspectableValue(value)}</pre>
    </div>
  );
}

function formatInspectableValue(value: unknown) {
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

function durationOptions(capability: ModelCapability | undefined) {
  const durations = capability?.limits.durations_sec;
  if (
    Array.isArray(durations) &&
    durations.every((duration) => typeof duration === "number")
  ) {
    return durations;
  }
  return [4, 5, 8];
}

function PanelHeader({ eyebrow, title }: { eyebrow: string; title: string }) {
  return (
    <div className="property-panel-header">
      <p>{eyebrow}</p>
      <h2>{title}</h2>
    </div>
  );
}

function EditablePanelHeader({
  eyebrow,
  isEditing,
  isUpdating,
  title,
  onCancel,
  onCommit,
  onEdit,
  onTitleChange,
}: {
  eyebrow: string;
  isEditing: boolean;
  isUpdating: boolean;
  title: string;
  onCancel: () => void;
  onCommit: () => void;
  onEdit: () => void;
  onTitleChange: (title: string) => void;
}) {
  return (
    <div className="property-panel-header property-panel-header-editable">
      <p>{eyebrow}</p>
      {isEditing ? (
        <input
          autoFocus
          disabled={isUpdating}
          onBlur={onCommit}
          onChange={(event) => onTitleChange(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onCommit();
            }
            if (event.key === "Escape") {
              event.preventDefault();
              onCancel();
            }
          }}
          value={title}
        />
      ) : (
        <button
          className="property-title-edit-button"
          onDoubleClick={onEdit}
          title="双击编辑标题"
          type="button"
        >
          {title}
        </button>
      )}
    </div>
  );
}
