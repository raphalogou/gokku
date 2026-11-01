import { expect, test } from "@playwright/test";
import { signIn, watchErrors } from "./helpers";

// Runs after domains.spec.ts. Restarting blog-api leaves it as it was.
test("an action keeps you on the page and shows the job in the dock (JOB-3)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps/blog-api/scaling");
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(page).toHaveURL(/\/apps\/blog-api\/scaling$/);

  const dock = page.getByRole("region", { name: "Jobs" });
  await expect(dock.getByText("Restart blog-api")).toBeVisible();
  await expect(dock.getByText(/^Succeeded/)).toBeVisible({ timeout: 15_000 });

  // Minimize to a pill, then bring it back.
  await dock.getByRole("button", { name: "Minimize jobs" }).click();
  await expect(dock.getByRole("button", { name: "Show jobs" })).toContainText("1 finished");
  await dock.getByRole("button", { name: "Show jobs" }).click();

  // The full page, then Back to where the job was started.
  await dock.getByRole("link", { name: "Open Restart blog-api in full page" }).click();
  await expect(page).toHaveURL(/\/jobs\/[0-9A-Z]+$/);
  await expect(page.getByRole("heading", { level: 1, name: "Restart blog-api" })).toBeVisible();
  await expect(dock.getByRole("button", { name: "Show jobs" })).toBeVisible();
  await page.getByRole("button", { name: "Back" }).click();
  await expect(page).toHaveURL(/\/apps\/blog-api\/scaling$/);

  await dock.getByRole("button", { name: "Dismiss Restart blog-api" }).click();
  await expect(dock).toHaveCount(0);
  expect(errors()).toEqual([]);
});

test("on a phone the dock sits above the tab bar", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page);
  await page.goto("/apps/blog-api");
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  const dock = page.getByRole("region", { name: "Jobs" });
  await expect(dock.getByText(/^Succeeded/)).toBeVisible({ timeout: 15_000 });
  const box = await dock.boundingBox();
  const tabs = await page.getByRole("navigation", { name: "Main" }).last().boundingBox();
  expect(box && tabs && box.y + box.height <= tabs.y).toBe(true);
  expect(box && box.x >= 0 && box.x + box.width <= 390).toBe(true);
  await dock.getByRole("button", { name: "Dismiss Restart blog-api" }).click();
});
