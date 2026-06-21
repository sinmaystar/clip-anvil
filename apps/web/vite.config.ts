import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const webPort = Number(process.env.CLIPANVIL_WEB_PORT ?? 5173);
const serverPort = Number(process.env.CLIPANVIL_SERVER_PORT ?? 8888);
const serverHost = process.env.CLIPANVIL_SERVER_HOST ?? "localhost";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    chunkSizeWarningLimit: 1800,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) {
            return;
          }
          if (id.includes("/tldraw/") || id.includes("/@tldraw/")) {
            return "vendor-tldraw";
          }
          if (
            id.includes("/react-markdown/") ||
            id.includes("/remark-gfm/") ||
            id.includes("/micromark") ||
            id.includes("/mdast") ||
            id.includes("/hast") ||
            id.includes("/unified/")
          ) {
            return "vendor-markdown";
          }
          if (id.includes("/@tanstack/react-query/")) {
            return "vendor-query";
          }
          if (
            id.includes("/react/") ||
            id.includes("/react-dom/") ||
            id.includes("/react-router/")
          ) {
            return "vendor-react";
          }
        },
      },
    },
  },
  server: {
    port: webPort,
    host: true,
    allowedHosts: true,
    proxy: {
      "/api": `http://${serverHost}:${serverPort}`,
      "/ws": {
        target: `ws://${serverHost}:${serverPort}`,
        ws: true,
      },
    },
  },
});
