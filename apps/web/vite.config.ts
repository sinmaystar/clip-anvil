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
