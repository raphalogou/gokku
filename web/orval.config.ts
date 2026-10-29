import { defineConfig } from "orval";

// Generated from api/openapi.yaml by `task gen`. Do not edit the generated files.
export default defineConfig({
  client: {
    input: "../api/openapi.yaml",
    output: {
      target: "src/lib/api/gen/client.ts",
      schemas: "src/lib/api/gen/model",
      client: "fetch",
      mode: "single",
      baseUrl: "/api/v1",
      clean: true,
      override: {
        mutator: { path: "src/lib/api/fetcher.ts", name: "apiFetch" },
        fetch: { includeHttpResponseReturnType: false },
      },
    },
  },
  zod: {
    input: "../api/openapi.yaml",
    output: {
      target: "src/lib/api/gen/zod.ts",
      client: "zod",
      mode: "single",
    },
  },
});
