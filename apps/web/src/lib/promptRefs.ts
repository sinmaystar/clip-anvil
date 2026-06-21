import type { MediaEdge, MediaNode, PromptRef, PromptRefsDocument } from "./api";

type PromptRefNode = Pick<MediaNode, "id" | "title" | "node_type">;

export interface InputReferenceState {
  explicit: Array<{ node: MediaNode; ref: PromptRef }>;
  implicit: MediaNode[];
  invalid: Array<{ ref: PromptRef }>;
}

export function normalizePromptRefs(value: unknown): PromptRefsDocument {
  if (!value || Array.isArray(value)) {
    return { version: 1, refs: [] };
  }
  if (typeof value !== "object") {
    return { version: 1, refs: [] };
  }
  const candidate = value as Partial<PromptRefsDocument>;
  const refs = Array.isArray(candidate.refs) ? candidate.refs : [];
  return {
    version: 1,
    refs: normalizeRefs(refs),
  };
}

export function upstreamNodesForTarget(
  target: Pick<MediaNode, "id">,
  nodes: MediaNode[],
  edges: MediaEdge[],
) {
  const upstreamIDs = edges
    .filter((edge) => edge.to_node_id === target.id)
    .map((edge) => edge.from_node_id);
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return upstreamIDs
    .map((nodeID) => byID.get(nodeID))
    .filter((node): node is MediaNode => Boolean(node));
}

export function candidatePromptRefNodes(
  target: Pick<MediaNode, "id">,
  nodes: MediaNode[],
  edges: MediaEdge[],
) {
  const upstream = upstreamNodesForTarget(target, nodes, edges);
  const upstreamIDs = new Set(upstream.map((node) => node.id));
  const rest = nodes.filter(
    (node) => node.id !== target.id && !upstreamIDs.has(node.id),
  );
  return [...upstream, ...rest];
}

export function buildPromptRefDocument(
  nodeIds: string[],
  nodes: PromptRefNode[],
): PromptRefsDocument {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const seen = new Set<string>();
  const refs: PromptRef[] = [];

  for (const nodeID of nodeIds) {
    if (seen.has(nodeID)) {
      continue;
    }
    const node = byID.get(nodeID);
    if (!node) {
      continue;
    }
    seen.add(nodeID);
    refs.push({
      node_id: node.id,
      label: node.title,
      node_type: node.node_type,
    });
  }

  return { version: 1, refs };
}

export function promptRefsAfterSelect(
  current: unknown,
  node: PromptRefNode,
): PromptRefsDocument {
  const refs = normalizePromptRefs(current).refs;
  if (refs.some((ref) => ref.node_id === node.id)) {
    return { version: 1, refs };
  }
  return {
    version: 1,
    refs: [
      ...refs,
      {
        node_id: node.id,
        label: node.title || "未命名",
        node_type: node.node_type,
      },
    ],
  };
}

export function promptRefsAfterRemove(
  current: unknown,
  nodeID: string,
): PromptRefsDocument {
  return {
    version: 1,
    refs: normalizePromptRefs(current).refs.filter(
      (ref) => ref.node_id !== nodeID,
    ),
  };
}

export function inputReferenceState(
  target: MediaNode,
  nodes: MediaNode[],
  edges: MediaEdge[],
): InputReferenceState {
  const refs = normalizePromptRefs(target.prompt_refs).refs;
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const upstream = upstreamNodesForTarget(target, nodes, edges);
  const upstreamIDs = new Set(upstream.map((node) => node.id));
  const explicit: InputReferenceState["explicit"] = [];
  const invalid: InputReferenceState["invalid"] = [];
  const explicitIDs = new Set<string>();

  for (const ref of refs) {
    const node = nodesByID.get(ref.node_id);
    if (!node || !upstreamIDs.has(ref.node_id)) {
      invalid.push({ ref });
      continue;
    }
    explicitIDs.add(ref.node_id);
    explicit.push({ node, ref });
  }

  return {
    explicit,
    implicit: upstream.filter((node) => !explicitIDs.has(node.id)),
    invalid,
  };
}

export function runDisabledReasonForPromptRefs(
  state: Pick<InputReferenceState, "invalid">,
) {
  return state.invalid.length > 0
    ? "Prompt 中有失效引用，请重新选择或移除后再运行。"
    : null;
}

export function collectPromptRefMentions(
  prompt: string,
  nodes: PromptRefNode[],
): string[] {
  const mentions = new Set<string>();
  for (const node of nodes) {
    const title = node.title.trim();
    if (title && includesMention(prompt, title)) {
      mentions.add(node.id);
      continue;
    }
    if (includesMention(prompt, node.id)) {
      mentions.add(node.id);
    }
  }
  return Array.from(mentions);
}

export function promptRefNodeIds(value: unknown): string[] {
  return normalizePromptRefs(value).refs.map((ref) => ref.node_id);
}

export function isExplicitPromptRef(
  value: unknown,
  nodeID: string,
): boolean {
  return promptRefNodeIds(value).includes(nodeID);
}

export function summarizePromptRefs(
  value: unknown,
  upstreamNodes: PromptRefNode[],
): { explicit: PromptRefNode[]; implicit: PromptRefNode[] } {
  const explicitIDs = new Set(promptRefNodeIds(value));
  return {
    explicit: upstreamNodes.filter((node) => explicitIDs.has(node.id)),
    implicit: upstreamNodes.filter((node) => !explicitIDs.has(node.id)),
  };
}

export function promptRefRenamePatch(
  target: Pick<MediaNode, "prompt" | "prompt_refs">,
  renamedNode: PromptRefNode,
): {
  prompt: string;
  prompt_refs: PromptRefsDocument;
  prompt_rich: { version: 1; source: "textarea-at"; text: string };
} | null {
  const doc = normalizePromptRefs(target.prompt_refs);
  let changed = false;
  let prompt = target.prompt;
  const refs = doc.refs.map((ref) => {
    if (ref.node_id !== renamedNode.id) {
      return ref;
    }
    const nextLabel = renamedNode.title || "未命名";
    if (ref.label && ref.label !== nextLabel) {
      prompt = rewriteMentionLabel(prompt, ref.label, nextLabel);
    }
    if (ref.label !== nextLabel || ref.node_type !== renamedNode.node_type) {
      changed = true;
      return {
        node_id: ref.node_id,
        label: nextLabel,
        node_type: renamedNode.node_type,
      };
    }
    return ref;
  });

  if (!changed && prompt === target.prompt) {
    return null;
  }

  return {
    prompt,
    prompt_refs: { version: 1, refs },
    prompt_rich: { version: 1, source: "textarea-at", text: prompt },
  };
}

function normalizeRefs(refs: unknown[]): PromptRef[] {
  const out: PromptRef[] = [];
  const seen = new Set<string>();
  for (const ref of refs) {
    if (!ref || typeof ref !== "object") {
      continue;
    }
    const candidate = ref as Partial<PromptRef>;
    if (
      !candidate.node_id ||
      typeof candidate.node_id !== "string" ||
      seen.has(candidate.node_id)
    ) {
      continue;
    }
    seen.add(candidate.node_id);
    out.push({
      node_id: candidate.node_id,
      label: typeof candidate.label === "string" ? candidate.label : "",
      node_type:
        typeof candidate.node_type === "string" ? candidate.node_type : "",
    });
  }
  return out;
}

function includesMention(prompt: string, value: string): boolean {
  const escaped = value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(
    `(^|[^\\p{L}\\p{N}_-])@${escaped}(?=$|[^\\p{L}\\p{N}_-])`,
    "u",
  ).test(prompt);
}

function rewriteMentionLabel(
  prompt: string,
  oldLabel: string,
  newLabel: string,
) {
  const escaped = oldLabel.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return prompt.replace(
    new RegExp(
      `(^|[^\\p{L}\\p{N}_-])@${escaped}(?=$|[^\\p{L}\\p{N}_-])`,
      "gu",
    ),
    `$1@${newLabel}`,
  );
}
