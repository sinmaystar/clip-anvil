import { useCallback, useEffect, useRef, useState } from "react";
import {
  createMediaNode,
  uploadMediaAsset,
  type MediaAsset,
  type MediaNode,
} from "../lib/api";

interface FileDropZoneProps {
  isUploading: boolean;
  onUploadFiles: (files: File[], point: { x: number; y: number }) => void;
  screenToCanvasPoint: (point: { x: number; y: number }) => {
    x: number;
    y: number;
  } | null;
  uploadError: string | null;
}

interface UseCanvasFileUploadOptions {
  workspaceId: string;
  onAssetNodeCreated: (node: MediaNode) => void;
}

export function useCanvasFileUpload({
  workspaceId,
  onAssetNodeCreated,
}: UseCanvasFileUploadOptions) {
  const errorTimerRef = useRef<number | undefined>(undefined);
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
      const uploadableFiles = files.filter(isSupportedUploadFile);
      if (uploadableFiles.length === 0) {
        setUploadError("仅支持图片、视频、音频文件");
        errorTimerRef.current = window.setTimeout(() => {
          setUploadError(null);
        }, 2800);
        return;
      }
      setIsUploading(true);
      setUploadError(null);
      window.clearTimeout(errorTimerRef.current);
      try {
        const uploads = await Promise.allSettled(
          uploadableFiles.map((file) => uploadMediaAsset(workspaceId, file)),
        );
        const failedUploads = uploads.filter(
          (result) => result.status === "rejected",
        ).length;
        const created = await Promise.allSettled(
          uploads.map((result, index) => {
            if (result.status !== "fulfilled") {
              return Promise.resolve();
            }
            return createNodeForAsset(
              result.value,
              uploadableFiles[index],
              point,
              index,
            );
          }),
        );
        const failedCreates = created.filter(
          (result) => result.status === "rejected",
        ).length;
        const failures = failedUploads + failedCreates;
        if (failures > 0) {
          setUploadError(
            failures === uploadableFiles.length
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

  useEffect(
    () => () => {
      window.clearTimeout(errorTimerRef.current);
    },
    [],
  );

  return {
    isUploading,
    uploadError,
    uploadFiles,
  };
}

export function FileDropZone({
  isUploading,
  onUploadFiles,
  screenToCanvasPoint,
  uploadError,
}: FileDropZoneProps) {
  const [isActive, setIsActive] = useState(false);

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
      event.stopPropagation();
      setIsActive(true);
    };

    const onDragOver = (event: DragEvent) => {
      if (!hasFiles(event)) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
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
      event.stopPropagation();
      event.stopImmediatePropagation();
      resetDragState();
      if (!event.dataTransfer) {
        return;
      }
      const files = Array.from(event.dataTransfer.files);
      const point = screenToCanvasPoint({
        x: event.clientX,
        y: event.clientY,
      });
      if (point) {
        onUploadFiles(files, point);
      }
    };

    window.addEventListener("dragenter", onDragEnter, { capture: true });
    window.addEventListener("dragover", onDragOver, { capture: true });
    window.addEventListener("dragleave", onDragLeave, { capture: true });
    window.addEventListener("drop", onDrop, { capture: true });
    window.addEventListener("dragend", resetDragState);
    window.addEventListener("blur", resetDragState);
    return () => {
      window.removeEventListener("dragenter", onDragEnter, { capture: true });
      window.removeEventListener("dragover", onDragOver, { capture: true });
      window.removeEventListener("dragleave", onDragLeave, { capture: true });
      window.removeEventListener("drop", onDrop, { capture: true });
      window.removeEventListener("dragend", resetDragState);
      window.removeEventListener("blur", resetDragState);
    };
  }, [onUploadFiles, screenToCanvasPoint]);

  return (
    <div
      aria-live="polite"
      className="file-drop-zone"
      data-active={isActive || isUploading || Boolean(uploadError)}
    >
      <div className="file-drop-zone-panel">
        {uploadError ??
          (isUploading ? "正在上传素材" : "释放后创建素材节点")}
      </div>
    </div>
  );
}

function isSupportedUploadFile(file: File) {
  return (
    file.type.startsWith("image/") ||
    file.type.startsWith("video/") ||
    file.type.startsWith("audio/")
  );
}

function fileNameWithoutExtension(name: string) {
  const trimmed = name.trim();
  if (!trimmed) {
    return "未命名素材";
  }
  return trimmed.replace(/\.[^.]+$/, "") || trimmed;
}
