import type { CanvasPayload, MediaNode, ProductionPreview } from "./api";

export function preserveCanvasAssetUrls(
  previous: CanvasPayload | undefined,
  next: CanvasPayload,
): CanvasPayload {
  if (!previous) {
    return next;
  }

  const previousNodes = new Map(previous.nodes.map((node) => [node.id, node]));
  let changed = false;
  const nodes = next.nodes.map((node) => {
    const previousNode = previousNodes.get(node.id);
    if (!previousNode) {
      return node;
    }
    const stableNode = preserveNodeAssetUrls(previousNode, node);
    if (stableNode !== node) {
      changed = true;
    }
    return stableNode;
  });

  return changed ? { ...next, nodes } : next;
}

function preserveNodeAssetUrls(previous: MediaNode, next: MediaNode): MediaNode {
  let changed = false;
  let stableNode = next;

  if (
    previous.asset_id &&
    next.asset_id === previous.asset_id &&
    previous.thumbnail_url &&
    next.thumbnail_url &&
    previous.thumbnail_url !== next.thumbnail_url
  ) {
    stableNode = { ...stableNode, thumbnail_url: previous.thumbnail_url };
    changed = true;
  }

  const productionPreview = preserveProductionPreviewUrls(
    previous.production_preview,
    next.production_preview,
  );
  if (productionPreview !== next.production_preview) {
    stableNode = changed ? stableNode : { ...stableNode };
    stableNode.production_preview = productionPreview;
  }

  return stableNode;
}

function preserveProductionPreviewUrls(
  previous: ProductionPreview | undefined,
  next: ProductionPreview | undefined,
) {
  if (!previous || !next || !sameProductionPreviewIdentity(previous, next)) {
    return next;
  }

  let changed = false;
  let stablePreview = next;
  if (
    previous.access_url &&
    next.access_url &&
    previous.access_url !== next.access_url
  ) {
    stablePreview = { ...stablePreview, access_url: previous.access_url };
    changed = true;
  }

  if (
    previous.thumbnail_url &&
    next.thumbnail_url &&
    previous.thumbnail_url !== next.thumbnail_url
  ) {
    stablePreview = changed ? stablePreview : { ...stablePreview };
    stablePreview.thumbnail_url = previous.thumbnail_url;
  }

  return stablePreview;
}

function sameProductionPreviewIdentity(
  previous: ProductionPreview,
  next: ProductionPreview,
) {
  return (
    previous.version_id === next.version_id &&
    previous.version_no === next.version_no &&
    nullableValue(previous.asset_id) === nullableValue(next.asset_id) &&
    nullableValue(previous.asset_type) === nullableValue(next.asset_type) &&
    nullableValue(previous.mime) === nullableValue(next.mime) &&
    nullableValue(previous.width) === nullableValue(next.width) &&
    nullableValue(previous.height) === nullableValue(next.height) &&
    nullableValue(previous.duration_ms) === nullableValue(next.duration_ms) &&
    nullableValue(previous.input_hash) === nullableValue(next.input_hash)
  );
}

function nullableValue(value: string | number | undefined) {
  return value ?? null;
}
