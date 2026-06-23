import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type PointerEvent,
} from "react";
import "tldraw/tldraw.css";
import {
  atom,
  createTLStore,
  defaultAssetUtils,
  defaultBindingUtils,
  defaultShapeUtils,
  Editor,
  StateNode,
  Tldraw,
  type TLArrowBinding,
  type TLBindingCreate,
  type TLRecord,
  type TLShapePartial,
  type TLUiComponents,
} from "tldraw";
import type { CanvasPayload } from "../../lib/api";
import {
  edgeToArrow,
  groupToShape,
  nodeToShape,
  shapeIdForEdge,
  shapeIdForGroup,
  shapeIdForNode,
} from "../../lib/canvas";
import { AgentReadonlyMediaShapeUtil } from "../../shapes/AgentReadonlyMediaShapeUtil";
import { GroupContainerShapeUtil } from "../../shapes/GroupContainerShapeUtil";

const readonlyTldrawComponents: TLUiComponents = {
  ContextMenu: null,
  ActionsMenu: null,
  HelpMenu: null,
  ZoomMenu: null,
  MainMenu: null,
  Minimap: null,
  StylePanel: null,
  PageMenu: null,
  NavigationPanel: null,
  Toolbar: null,
  RichTextToolbar: null,
  ImageToolbar: null,
  VideoToolbar: null,
  KeyboardShortcutsDialog: null,
  QuickActions: null,
  HelperButtons: null,
  DebugPanel: null,
  DebugMenu: null,
  MenuPanel: null,
  TopPanel: null,
  SharePanel: null,
  Dialogs: null,
  Toasts: null,
};

const readonlyCollaborationMode = atom<"readonly" | "readwrite">(
  "clip-anvil-agent-readonly-collaboration-mode",
  "readonly",
);
const readonlyCollaborationStatus = atom<"offline" | "online">(
  "clip-anvil-agent-readonly-collaboration-status",
  "online",
);

class AgentReadonlyTool extends StateNode {
  static override id = "agent_readonly";
  static override isLockable = false;
}

const readonlyTldrawTools = [AgentReadonlyTool] as const;

export function AgentReadonlyCanvas({
  canvas,
  onSelectNode,
  selectedNodeId,
}: {
  canvas: CanvasPayload;
  onSelectNode: (nodeId: string | null) => void;
  selectedNodeId: string | null;
}) {
  const [editor, setEditor] = useState<Editor | null>(null);
  const readonlyCanvasRootRef = useRef<HTMLDivElement | null>(null);
  const lastFitSignatureRef = useRef("");
  const shapeUtils = useMemo(
    () => [...defaultShapeUtils, GroupContainerShapeUtil, AgentReadonlyMediaShapeUtil],
    [],
  );
  const store = useMemo(
    () =>
      createTLStore({
        assetUtils: defaultAssetUtils,
        bindingUtils: defaultBindingUtils,
        shapeUtils,
        collaboration: {
          mode: readonlyCollaborationMode,
          status: readonlyCollaborationStatus,
        },
      }),
    [shapeUtils],
  );

  const handleMount = useCallback(
    (mountedEditor: Editor) => {
      setEditor(mountedEditor);
      mountedEditor.setCurrentTool("agent_readonly");
      mountedEditor.user.updateUserPreferences({ edgeScrollSpeed: 0 });
      mountedEditor.setCamera({
        x: canvas.camera.x,
        y: canvas.camera.y,
        z: canvas.camera.zoom,
      });
      syncReadonlyEditorWithCanvas(mountedEditor, canvas, lastFitSignatureRef);
    },
    [canvas],
  );

  useEffect(() => {
    if (!editor) {
      return;
    }
    syncReadonlyEditorWithCanvas(editor, canvas, lastFitSignatureRef);
  }, [canvas, editor]);

  useEffect(() => {
    const onNodeSelect = (event: Event) => {
      const nodeId = (
        event as CustomEvent<{
          nodeId?: string;
        }>
      ).detail?.nodeId;
      if (nodeId) {
        onSelectNode(nodeId);
      }
    };

    window.addEventListener("clip-anvil:select-node", onNodeSelect);
    return () => {
      window.removeEventListener("clip-anvil:select-node", onNodeSelect);
    };
  }, [onSelectNode]);

  useEffect(() => {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:active-node-changed", {
        detail: { nodeId: selectedNodeId },
      }),
    );
  }, [selectedNodeId]);

  const stopReadonlyNodeTldrawGesture = useCallback(
    (event: Event | MouseEvent<HTMLDivElement> | PointerEvent<HTMLDivElement>) => {
      if (
        event.target instanceof Element &&
        event.target.closest(".agent-readonly-media-node-shell")
      ) {
        event.stopPropagation();
        const nativeEvent = "nativeEvent" in event ? event.nativeEvent : event;
        nativeEvent.stopImmediatePropagation();
      }
    },
    [],
  );

  useEffect(() => {
    const root = readonlyCanvasRootRef.current;
    if (!root) {
      return;
    }
    const stopNativeReadonlyNodeGesture = (event: Event) => {
      stopReadonlyNodeTldrawGesture(event);
    };
    root.addEventListener("pointerdown", stopNativeReadonlyNodeGesture, true);
    root.addEventListener("mousedown", stopNativeReadonlyNodeGesture, true);
    return () => {
      root.removeEventListener(
        "pointerdown",
        stopNativeReadonlyNodeGesture,
        true,
      );
      root.removeEventListener(
        "mousedown",
        stopNativeReadonlyNodeGesture,
        true,
      );
    };
  }, [stopReadonlyNodeTldrawGesture]);

  return (
    <div
      className="agent-readonly-tldraw"
      onMouseDownCapture={stopReadonlyNodeTldrawGesture}
      onPointerDownCapture={stopReadonlyNodeTldrawGesture}
      ref={readonlyCanvasRootRef}
    >
      <Tldraw
        components={readonlyTldrawComponents}
        initialState="agent_readonly"
        onMount={handleMount}
        options={{
          edgeScrollSpeed: 0,
          enableToolbarKeyboardShortcuts: false,
        }}
        shapeUtils={shapeUtils}
        store={store}
        tools={readonlyTldrawTools}
      />
    </div>
  );
}

function syncReadonlyEditorWithCanvas(
  editor: Editor,
  canvas: CanvasPayload,
  lastFitSignatureRef: { current: string },
) {
  const groupShapes = canvas.groups.map((group) =>
    lockReadonlyShape(groupToShape(group, canvas.nodes)),
  );
  const nodeShapes = canvas.nodes.map((node) => lockReadonlyShape(nodeToShape(node)));
  const edgeShapes: TLShapePartial[] = [];
  const edgeBindings: TLBindingCreate<TLArrowBinding>[] = [];
  for (const edge of canvas.edges) {
    const arrow = edgeToArrow(edge, canvas.nodes);
    if (arrow) {
      edgeShapes.push(lockReadonlyShape(arrow.arrow as TLShapePartial));
      edgeBindings.push(...arrow.bindings);
    }
  }
  const canvasShapes: TLShapePartial[] = [
    ...groupShapes,
    ...edgeShapes,
    ...nodeShapes,
  ];
  const desiredShapeIds = new Set([
    ...canvas.groups.map((group) => shapeIdForGroup(group.id)),
    ...canvas.edges.map((edge) => shapeIdForEdge(edge.id)),
    ...canvas.nodes.map((node) => shapeIdForNode(node.id)),
  ]);

  withReadonlyStoreWrite(() => {
    editor.run(() => {
      editor.store.mergeRemoteChanges(() => {
        const existingPageShapes = editor.getCurrentPageShapes();
        const staleShapeIds = existingPageShapes
          .filter((shape) => {
            if (
              shape.type !== "media" &&
              shape.type !== "group-container" &&
              !edgeIdFromRecord(shape)
            ) {
              return false;
            }
            return !desiredShapeIds.has(shape.id);
          })
          .map((shape) => shape.id);

        if (staleShapeIds.length > 0) {
          editor.deleteShapes(staleShapeIds);
        }

        const shapesToCreate = canvasShapes.filter(
          (shape) => !editor.getShape(shape.id),
        );
        const shapesToUpdate = canvasShapes.filter((shape) => {
          const existing = editor.getShape(shape.id);
          return existing && readonlyShapeChanged(existing, shape);
        });

        if (shapesToCreate.length > 0) {
          editor.createShapes(shapesToCreate);
        }
        if (shapesToUpdate.length > 0) {
          editor.updateShapes(shapesToUpdate);
        }
        for (const binding of edgeBindings) {
          syncReadonlyArrowBinding(editor, binding);
        }
      });
    }, {
      history: "ignore",
      ignoreShapeLock: true,
    });
  });
  editor.setCurrentTool("agent_readonly");
  fitReadonlyEditorToCanvas(editor, canvas, lastFitSignatureRef);
}

function fitReadonlyEditorToCanvas(
  editor: Editor,
  canvas: CanvasPayload,
  lastFitSignatureRef: { current: string },
) {
  if (canvas.nodes.length === 0) {
    lastFitSignatureRef.current = "";
    return;
  }
  const signature = readonlyFitSignature(canvas);
  if (signature === lastFitSignatureRef.current) {
    return;
  }
  lastFitSignatureRef.current = signature;
  window.requestAnimationFrame(() => {
    editor.zoomToFit({ animation: { duration: 0 } });
    editor.setCurrentTool("agent_readonly");
  });
}

function readonlyFitSignature(canvas: CanvasPayload) {
  return canvas.nodes
    .map((node) =>
      [
        node.id,
        node.canvas_x,
        node.canvas_y,
        node.canvas_w,
        node.canvas_h,
      ].join(":"),
    )
    .sort()
    .join("|");
}

function readonlyShapeChanged(existing: TLRecord, desired: TLShapePartial) {
  return Object.entries(desired).some(([key, value]) => {
    const existingValue = (existing as unknown as Record<string, unknown>)[key];
    return (
      JSON.stringify(readonlyComparableShapeValue(key, existingValue)) !==
      JSON.stringify(readonlyComparableShapeValue(key, value))
    );
  });
}

function readonlyComparableShapeValue(key: string, value: unknown): unknown {
  if (key === "props" && value && typeof value === "object") {
    const props = { ...(value as Record<string, unknown>) };
    for (const assetURLKey of [
      "previewAssetUrl",
      "previewThumbnailUrl",
      "thumbnailUrl",
    ]) {
      props[assetURLKey] = stableReadonlyAssetURL(props[assetURLKey]);
    }
    return props;
  }
  return value;
}

function stableReadonlyAssetURL(value: unknown) {
  if (typeof value !== "string" || value.trim() === "") {
    return value;
  }
  try {
    const url = new URL(value, window.location.origin);
    url.search = "";
    url.hash = "";
    return url.toString();
  } catch {
    return value;
  }
}

function syncReadonlyArrowBinding(
  editor: Editor,
  binding: TLBindingCreate<TLArrowBinding>,
) {
  const bindingProps = binding.props as TLArrowBinding["props"] | undefined;
  if (!bindingProps) {
    return;
  }
  const existingMany = editor
    .getBindingsFromShape(binding.fromId, "arrow")
    .filter(
      (item) =>
        item.type === "arrow" &&
        item.props.terminal === bindingProps.terminal,
    );
  if (existingMany.length > 1) {
    editor.deleteBindings(existingMany.slice(1));
  }
  const existing = existingMany[0];
  if (!existing) {
    editor.createBinding(binding);
    return;
  }
  const nextBinding = {
    ...existing,
    toId: binding.toId,
    props: bindingProps,
  };
  if (readonlyBindingChanged(existing, nextBinding)) {
    editor.updateBinding(nextBinding);
  }
}

function readonlyBindingChanged(existing: TLArrowBinding, desired: TLArrowBinding) {
  return (
    existing.toId !== desired.toId ||
    JSON.stringify(existing.props) !== JSON.stringify(desired.props)
  );
}

function lockReadonlyShape(shape: TLShapePartial): TLShapePartial {
  return { ...shape, isLocked: true };
}

function withReadonlyStoreWrite(write: () => void) {
  readonlyCollaborationMode.set("readwrite");
  try {
    write();
  } finally {
    readonlyCollaborationMode.set("readonly");
  }
}

function edgeIdFromRecord(record: TLRecord) {
  if (record.typeName !== "shape" || !("meta" in record)) {
    return null;
  }
  const edgeId = (record as { meta?: { edgeId?: unknown } }).meta?.edgeId;
  return typeof edgeId === "string" ? edgeId : null;
}
