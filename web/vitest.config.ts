import { defineConfig } from "vitest/config";

// Separate from vite.config.ts: the React Router plugin is for app builds, not unit tests.
export default defineConfig({
  resolve: { tsconfigPaths: true },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/test-setup.ts"],
  },
});
