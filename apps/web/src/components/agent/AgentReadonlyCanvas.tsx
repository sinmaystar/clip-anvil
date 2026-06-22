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
      syncReadonlyEditorWithCanvas(mountedEditor, canvas);
    },
    [canvas],
  );

  useEffect(() => {
    if (!editor) {
      return;
    }
    syncReadonlyEditorWithCanvas(editor, canvas);
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

function syncReadonlyEditorWithCanvas(editor: Editor, canvas: CanvasPayload) {
  const groupShapes = canvas.groups.map((group) =>
    lockReadonlyShape(groupToShape(group, canvas.nodes)),
  );
  const nodeShapes = canvas.nodes.map((node) => lockReadonlyShape(nodeToShape(node)));
  const edgeShapes: TLShapePartial[] = [];
  for (const edge of canvas.edges) {
    const arrow = edgeToArrow(edge, canvas.nodes);
    if (arrow) {
      edgeShapes.push(lockReadonlyShape(arrow.arrow as TLShapePartial));
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
      const shapesToUpdate = canvasShapes.filter((shape) =>
        editor.getShape(shape.id),
      );

      if (shapesToCreate.length > 0) {
        editor.createShapes(shapesToCreate);
      }
      if (shapesToUpdate.length > 0) {
        editor.updateShapes(shapesToUpdate);
      }
    });
  });
  editor.setCurrentTool("agent_readonly");
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
