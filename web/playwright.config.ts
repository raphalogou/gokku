import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run against the real binary built by `task build`, with the fake runner
// and a fresh data directory. The account tests build on each other, so files run in order.
const port = 18080;

// Set to use a system Chromium (for example /usr/bin/ungoogled-chromium) instead of the
// browser downloaded by `npx playwright install chromium`.
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined;

export default defineConfig({
  testDir: "e2e",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: `http://127.0.0.1:${port}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], launchOptions: { executablePath } } }],
  webServer: {
    command: `rm -rf e2e/.data && ../bin/gokku serve --listen 127.0.0.1:${port} --data-dir e2e/.data`,
    url: `http://127.0.0.1:${port}/healthz`,
    env: { GOKKU_RUNNER: "fake", GOKKU_LOG_LEVEL: "warn" },
    reuseExistingServer: false,
  },
});
