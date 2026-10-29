import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run against the real binary built by `task build`, with the fake runner.
const port = 18080;

// Set to use a system Chromium (for example /usr/bin/ungoogled-chromium) instead of the
// browser downloaded by `npx playwright install chromium`.
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined;

export default defineConfig({
  testDir: "e2e",
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: `http://127.0.0.1:${port}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], launchOptions: { executablePath } } }],
  webServer: {
    command: `../bin/gokku serve --listen 127.0.0.1:${port}`,
    url: `http://127.0.0.1:${port}/healthz`,
    env: { GOKKU_RUNNER: "fake", GOKKU_LOG_LEVEL: "warn" },
    reuseExistingServer: false,
  },
});
