import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface MarkdownPreviewProps {
  value: string;
  variant: "canvas" | "panel";
}

export function MarkdownPreview({ value, variant }: MarkdownPreviewProps) {
  return (
    <div className="markdown-preview" data-variant={variant}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
        {value}
      </ReactMarkdown>
    </div>
  );
}
