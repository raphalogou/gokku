import { reactRouter } from "@react-router/dev/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// In development Vite serves the app and forwards API calls to the Go server (`task dev`).
const api = process.env.GOKKU_API_URL ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [tailwindcss(), reactRouter()],
  resolve: { tsconfigPaths: true },
  // Never inline assets as data: URIs. The CSP allows fonts and scripts from 'self' only (SEC-6).
  build: { assetsInlineLimit: 0 },
  server: {
    proxy: {
      "/api": { target: api, changeOrigin: false },
      "/healthz": { target: api },
    },
  },
});
