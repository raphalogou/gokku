import { expect, test } from "@playwright/test";
import { signIn, watchErrors } from "./helpers";

// Runs after account.spec.ts and before projects.spec.ts, and leaves every app as it found
// it except status-page, whose failed rebuild keeps it failed as the mockup shows.
test.describe.configure({ mode: "serial" });

test("restart an app and watch the job output (APP-6, JOB-3)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps/blog-api");
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(page).toHaveURL(/\/jobs\/[0-9A-Z]+$/);
  await expect(page.getByRole("heading", { level: 1, name: "Restart blog-api" })).toBeVisible();
  const output = page.getByRole("log", { name: "Output" });
  await expect(output).toContainText("-----> Restarting blog-api");
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(output).toContainText("=====> Application deployed:");
  await expect(page.getByText("Finished")).toBeVisible();
  expect(errors()).toEqual([]);
});

test("stop and start ask, then change the app status (APP-6, APP-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site");
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Stop docs-site?" });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });

  await page.getByRole("link", { name: "docs-site" }).click();
  await expect(page.getByText("Stopped", { exact: true }).first()).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
  await page.getByRole("link", { name: "docs-site" }).click();
  await expect(page.getByText("Running", { exact: true }).first()).toBeVisible({ timeout: 10_000 });
});

test("a failed rebuild shows the error and fails the app (APP-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/status-page");
  await page.getByRole("button", { name: "Rebuild" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Rebuild" }).click();
  await expect(page.getByRole("alert")).toContainText("dokku ps:rebuild status-page: exit status 1", {
    timeout: 15_000,
  });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("ERROR");
  await page.getByRole("link", { name: "status-page" }).click();
  await expect(page.getByText("Failed", { exact: true }).first()).toBeVisible();
});

test("cancel a running job (JOB-4)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/blog-api");
  await page.getByRole("button", { name: "Rebuild" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Rebuild" }).click();
  await page.getByRole("button", { name: "Cancel job" }).click();
  await expect(page.getByText("Canceled", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("Canceled.");
});

test("logs tail, filter, follow and pause (LOG-1, LOG-2, LOG-5)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/blog-api/logs");
  const log = page.getByRole("log", { name: "Logs" });
  await expect(log).toContainText("web.1");
  await expect(page.getByText("Following output")).toBeVisible();

  await page.getByLabel("Process").click();
  await page.getByRole("option", { name: "worker.1" }).click();
  await expect(log).not.toContainText("web.2");
  await page.getByLabel("Filter lines").fill("digest");
  await expect(log).not.toContainText("cache:warm");

  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByText(/^Paused/)).toBeVisible();
  await expect(page.getByText(/Paused, \d+ new line/)).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: "Resume" }).click();
  await expect(page.getByText("Following output")).toBeVisible();

  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download" }).click();
  expect((await download).suggestedFilename()).toBe("blog-api-worker.1.log");
});

test("activity lists the jobs and exports CSV (ACT-1, ACT-2)", async ({ page }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Activity" }).click();
  const table = page.getByRole("table", { name: "Events" });
  await expect(table.getByRole("row", { name: /apps:rebuild.*status-page.*Failed/ }).first()).toBeVisible();
  await expect(table.getByRole("row", { name: /apps:restart.*blog-api.*Succeeded/ }).first()).toBeVisible();

  await page.getByLabel("Result").click();
  await page.getByRole("option", { name: "Failed" }).click();
  await expect(table.getByRole("row", { name: /Succeeded/ })).toHaveCount(0);

  const download = page.waitForEvent("download");
  await page.getByRole("link", { name: "Export CSV" }).click();
  expect((await download).suggestedFilename()).toMatch(/^gokku-audit-\d{4}-\d{2}-\d{2}\.csv$/);

  await table.getByRole("link", { name: "apps:rebuild" }).first().click();
  await expect(page).toHaveURL(/\/jobs\//);
});

test("an operator can restart but not export the audit log", async ({ page }) => {
  await signIn(page, "olga", "olga's long password");
  await page.goto("/apps/storefront");
  await expect(page.getByRole("button", { name: "Restart", exact: true })).toBeVisible();
  await page.goto("/activity");
  await expect(page.getByRole("table", { name: "Events" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Export CSV" })).toHaveCount(0);
});
