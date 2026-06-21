import test from "node:test";
import assert from "node:assert/strict";

import {
  buildPromptRefDocument,
  candidatePromptRefNodes,
  collectPromptRefMentions,
  inputReferenceState,
  normalizePromptRefs,
  promptRefsAfterSelect,
  promptRefRenamePatch,
  runDisabledReasonForPromptRefs,
} from "../../dist-test/lib/promptRefs.js";

const baseNode = {
  workspace_id: "workspace",
  prompt: "",
  status: "draft",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 320,
  canvas_h: 180,
  created_at: "now",
  updated_at: "now",
};
const target = {
  ...baseNode,
  id: "b",
  node_type: "image",
  title: "B",
  prompt_refs: { version: 1, refs: [] },
};
const a = { ...baseNode, id: "a", node_type: "image", title: "A" };
const c = { ...baseNode, id: "c", node_type: "text", title: "C" };
const pack = {
  ...baseNode,
  id: "p",
  node_type: "reference_pack",
  title: "Pack P",
};
const edgeAtoB = {
  id: "edge-ab",
  workspace_id: "workspace",
  from_node_id: "a",
  to_node_id: "b",
  edge_type: "dependency",
  source: "user",
  created_at: "now",
};

test("normalizePromptRefs defaults legacy empty arrays to an empty document", () => {
  assert.deepEqual(normalizePromptRefs([]), { version: 1, refs: [] });
});

test("candidatePromptRefNodes orders connected upstream candidates first", () => {
  assert.deepEqual(
    candidatePromptRefNodes(target, [target, c, a, pack], [edgeAtoB]).map(
      (node) => node.id,
    ),
    ["a", "c", "p"],
  );
});

test("candidatePromptRefNodes keeps source material nodes selectable", () => {
  const sourceText = {
    ...baseNode,
    id: "script",
    node_type: "text",
    title: "视频脚本",
    operation_type: "manual",
    status: "succeeded",
  };
  const sourceImage = {
    ...baseNode,
    id: "product",
    node_type: "image",
    title: "商品主图",
    operation_type: "upload",
    asset_id: "asset-1",
    status: "succeeded",
  };

  assert.deepEqual(
    candidatePromptRefNodes(
      target,
      [target, sourceImage, sourceText],
      [
        {
          ...edgeAtoB,
          id: "edge-script-b",
          from_node_id: "script",
        },
      ],
    ).map((node) => node.id),
    ["script", "product"],
  );
});

test("buildPromptRefDocument keeps first duplicate mention", () => {
  const nodes = [
    { id: "node-a", title: "First", node_type: "image" },
    { id: "node-b", title: "Second", node_type: "video" },
  ];

  assert.deepEqual(
    buildPromptRefDocument(["node-a", "node-a", "node-b"], nodes),
    {
      version: 1,
      refs: [
        { node_id: "node-a", label: "First", node_type: "image" },
        { node_id: "node-b", label: "Second", node_type: "video" },
      ],
    },
  );
});

test("promptRefsAfterSelect adds a selected ref once", () => {
  assert.deepEqual(promptRefsAfterSelect({ version: 1, refs: [] }, a).refs, [
    { node_id: "a", label: "A", node_type: "image" },
  ]);
  assert.deepEqual(
    promptRefsAfterSelect(
      { version: 1, refs: [{ node_id: "a", label: "A", node_type: "image" }] },
      a,
    ).refs,
    [{ node_id: "a", label: "A", node_type: "image" }],
  );
});

test("inputReferenceState classifies explicit implicit and invalid inputs", () => {
  const state = inputReferenceState(
    {
      ...target,
      prompt_refs: {
        version: 1,
        refs: [
          { node_id: "a", label: "A", node_type: "image" },
          { node_id: "missing", label: "Missing", node_type: "image" },
        ],
      },
    },
    [target, a, c],
    [
      edgeAtoB,
      {
        ...edgeAtoB,
        id: "edge-cb",
        from_node_id: "c",
      },
    ],
  );
  assert.deepEqual(
    state.explicit.map((item) => item.node.id),
    ["a"],
  );
  assert.deepEqual(
    state.implicit.map((node) => node.id),
    ["c"],
  );
  assert.deepEqual(
    state.invalid.map((item) => item.ref.node_id),
    ["missing"],
  );
});

test("runDisabledReasonForPromptRefs blocks running invalid refs", () => {
  assert.equal(
    runDisabledReasonForPromptRefs({
      invalid: [
        { ref: { node_id: "missing", label: "Missing", node_type: "image" } },
      ],
    }),
    "Prompt 中有失效引用，请重新选择或移除后再运行。",
  );
});

test("collectPromptRefMentions finds title and id @mentions", () => {
  const nodes = [
    { id: "node-a", title: "产品图", node_type: "image" },
    { id: "node-b", title: "Motion ref", node_type: "video" },
  ];

  assert.deepEqual(collectPromptRefMentions("@产品图 and @node-b", nodes), [
    "node-a",
    "node-b",
  ]);
});

test("collectPromptRefMentions does not match longer mention prefixes", () => {
  const nodes = [
    { id: "node-a", title: "Brand", node_type: "text" },
    { id: "node-b", title: "Branding", node_type: "text" },
  ];

  assert.deepEqual(collectPromptRefMentions("@Branding and @node-a2", nodes), [
    "node-b",
  ]);
});

test("promptRefRenamePatch updates mention text and ref label", () => {
  assert.deepEqual(
    promptRefRenamePatch(
      {
        prompt: "Use @Old Product, not @Old Productish",
        prompt_refs: {
          version: 1,
          refs: [{ node_id: "a", label: "Old Product", node_type: "image" }],
        },
      },
      { id: "a", title: "New Product", node_type: "image" },
    ),
    {
      prompt: "Use @New Product, not @Old Productish",
      prompt_refs: {
        version: 1,
        refs: [{ node_id: "a", label: "New Product", node_type: "image" }],
      },
      prompt_rich: {
        version: 1,
        source: "textarea-at",
        text: "Use @New Product, not @Old Productish",
      },
    },
  );
});
