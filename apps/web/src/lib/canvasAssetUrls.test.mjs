import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { preserveCanvasAssetUrls } from "../../dist-test/lib/canvasAssetUrls.js";

const baseCanvas = {
  camera: { x: 0, y: 0, zoom: 1 },
  edges: [],
  groups: [],
  nodes: [],
};

const baseNode = {
  id: "node-1",
  workspace_id: "workspace-1",
  node_type: "image",
  title: "Image",
  prompt: "",
  asset_id: "asset-1",
  status: "succeeded",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 360,
  canvas_h: 280,
  created_at: "2026-06-24T00:00:00Z",
  updated_at: "2026-06-24T00:00:00Z",
};

describe("canvas asset urls", () => {
  it("preserves existing signed urls when the preview identity did not change", () => {
    const previous = {
      ...baseCanvas,
      nodes: [
        {
          ...baseNode,
          thumbnail_url: "http://minio.local/thumb.png?X-Amz-Date=old",
          production_preview: {
            version_id: "version-1",
            version_no: 1,
            asset_id: "asset-1",
            asset_type: "image",
            access_url: "http://minio.local/image.png?X-Amz-Date=old",
            thumbnail_url: "http://minio.local/preview-thumb.png?X-Amz-Date=old",
            width: 1600,
            height: 900,
          },
        },
      ],
    };
    const next = {
      ...baseCanvas,
      nodes: [
        {
          ...baseNode,
          thumbnail_url: "http://minio.local/thumb.png?X-Amz-Date=new",
          production_preview: {
            version_id: "version-1",
            version_no: 1,
            asset_id: "asset-1",
            asset_type: "image",
            access_url: "http://minio.local/image.png?X-Amz-Date=new",
            thumbnail_url: "http://minio.local/preview-thumb.png?X-Amz-Date=new",
            width: 1600,
            height: 900,
          },
        },
      ],
    };

    const stable = preserveCanvasAssetUrls(previous, next);

    assert.equal(stable.nodes[0].thumbnail_url, previous.nodes[0].thumbnail_url);
    assert.equal(
      stable.nodes[0].production_preview.access_url,
      previous.nodes[0].production_preview.access_url,
    );
    assert.equal(
      stable.nodes[0].production_preview.thumbnail_url,
      previous.nodes[0].production_preview.thumbnail_url,
    );
  });

  it("uses new signed urls when the preview version changes", () => {
    const previous = {
      ...baseCanvas,
      nodes: [
        {
          ...baseNode,
          production_preview: {
            version_id: "version-1",
            version_no: 1,
            asset_id: "asset-1",
            access_url: "http://minio.local/image-old.png",
          },
        },
      ],
    };
    const next = {
      ...baseCanvas,
      nodes: [
        {
          ...baseNode,
          production_preview: {
            version_id: "version-2",
            version_no: 2,
            asset_id: "asset-2",
            access_url: "http://minio.local/image-new.png",
          },
        },
      ],
    };

    const stable = preserveCanvasAssetUrls(previous, next);

    assert.equal(
      stable.nodes[0].production_preview.access_url,
      next.nodes[0].production_preview.access_url,
    );
  });
});
