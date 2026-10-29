import type { Config } from "@react-router/dev/config";

// SPA mode: the Go binary serves build/client and falls back to index.html (DESIGN.md, 5.1).
export default {
  ssr: false,
  appDirectory: "src",
} satisfies Config;
