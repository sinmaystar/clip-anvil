import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "tldraw";
import {
  createMediaNode,
  uploadMediaAsset,
  type MediaAsset,
  type MediaNode,
} from "../lib/api";

interface FileDropZoneProps {
  editor: Editor | null;
  workspaceId: string;
  onAssetNodeCreated: (node: MediaNode) => void;
}

export function FileDropZone({
  editor,
  workspaceId,
  onAssetNodeCreated,
}: FileDropZoneProps) {
  const errorTimerRef = useRef<number | undefined>(undefined);
  const [isActive, setIsActive] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const createNodeForAsset = useCallback(
    async (
      asset: MediaAsset,
      file: File,
      point: { x: number; y: number },
      index: number,
    ) => {
      const node = await createMediaNode({
        workspace_id: workspaceId,
        node_type: asset.type,
        asset_id: asset.id,
        title: fileNameWithoutExtension(file.name),
        status: "succeeded",
        operation_type: "upload",
        canvas_x: point.x + index * 260,
        canvas_y: point.y,
      });
      onAssetNodeCreated({
        ...node,
        thumbnail_url: node.thumbnail_url ?? asset.thumbnail_url ?? asset.access_url,
      });
    },
    [onAssetNodeCreated, workspaceId],
  );

  const uploadFiles = useCallback(
    async (files: File[], point: { x: number; y: number }) => {
      if (files.length === 0) {
        return;
      }
      setIsUploading(true);
      setUploadError(null);
      window.clearTimeout(errorTimerRef.current);
      try {
        const uploads = await Promise.allSettled(
          files.map((file) => uploadMediaAsset(workspaceId, file)),
        );
        const failedUploads = uploads.filter(
          (result) => result.status === "rejected",
        ).length;
        const created = await Promise.allSettled(
          uploads.map((result, index) => {
            if (result.status !== "fulfilled") {
              return Promise.resolve();
            }
            return createNodeForAsset(result.value, files[index], point, index);
          }),
        );
        const failedCreates = created.filter(
          (result) => result.status === "rejected",
        ).length;
        const failures = failedUploads + failedCreates;
        if (failures > 0) {
          setUploadError(
            failures === files.length
              ? "上传失败，请确认素材服务可用"
              : `${failures} 个文件上传失败`,
          );
          errorTimerRef.current = window.setTimeout(() => {
            setUploadError(null);
          }, 2800);
        }
      } finally {
        setIsUploading(false);
      }
    },
    [createNodeForAsset, workspaceId],
  );

  useEffect(() => {
    const hasFiles = (event: DragEvent) =>
      Array.from(event.dataTransfer?.types ?? []).includes("Files");

    const resetDragState = () => {
      setIsActive(false);
    };

    const isOutsideWindow = (event: DragEvent) =>
      event.clientX <= 0 ||
      event.clientY <= 0 ||
      event.clientX >= window.innerWidth ||
      event.clientY >= window.innerHeight;

    const onDragEnter = (event: DragEvent) => {
      if (!hasFiles(event)) {
        return;
      }
      event.preventDefault();
      setIsActive(true);
    };

    const onDragOver = (event: DragEvent) => {
      if (!hasFiles(event)) {
        return;
      }
      event.preventDefault();
      event.dataTransfer!.dropEffect = "copy";
      setIsActive(true);
    };

    const onDragLeave = (event: DragEvent) => {
      if (!hasFiles(event)) {
        return;
      }
      if (event.relatedTarget === null || isOutsideWindow(event)) {
        setIsActive(false);
      }
    };

    const onDrop = (event: DragEvent) => {
      if (!hasFiles(event)) {
        return;
      }
      event.preventDefault();
      resetDragState();
      setUploadError(null);
      if (!editor || !event.dataTransfer) {
        return;
      }
      const files = Array.from(event.dataTransfer.files).filter((file) =>
        file.type.startsWith("image/") ||
        file.type.startsWith("video/") ||
        file.type.startsWith("audio/"),
      );
      const point = editor.screenToPage({
        x: event.clientX,
        y: event.clientY,
      });
      void uploadFiles(files, point);
    };

    window.addEventListener("dragenter", onDragEnter);
    window.addEventListener("dragover", onDragOver);
    window.addEventListener("dragleave", onDragLeave);
    window.addEventListener("drop", onDrop);
    window.addEventListener("dragend", resetDragState);
    window.addEventListener("blur", resetDragState);
    return () => {
      window.removeEventListener("dragenter", onDragEnter);
      window.removeEventListener("dragover", onDragOver);
      window.removeEventListener("dragleave", onDragLeave);
      window.removeEventListener("drop", onDrop);
      window.removeEventListener("dragend", resetDragState);
      window.removeEventListener("blur", resetDragState);
      window.clearTimeout(errorTimerRef.current);
    };
  }, [editor, uploadFiles]);

  return (
    <div
      aria-live="polite"
      className="file-drop-zone"
      data-active={isActive || isUploading || Boolean(uploadError)}
    >
      <div className="file-drop-zone-panel">
        {uploadError ?? (isUploading ? "上传中" : "释放文件上传")}
      </div>
    </div>
  );
}

function fileNameWithoutExtension(name: string) {
  const trimmed = name.trim();
  if (!trimmed) {
    return "未命名素材";
  }
  return trimmed.replace(/\.[^.]+$/, "") || trimmed;
}
