import { expect, test } from "@playwright/test";
import { openJob, signIn, watchErrors } from "./helpers";

// Runs after manage.spec.ts and before projects.spec.ts. status-page fails every rebuild in
// the fake runner, so it stays failed; the other apps end as they started.
test.describe.configure({ mode: "serial" });

test("restart every app of a project as one job (PRJ-5, JOB-5)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/projects/internal-tools");
  await page.getByRole("button", { name: "Restart all" }).click();
  await openJob(page);
  await expect(
    page.getByRole("heading", { level: 1, name: "Restart all apps in Internal tools" }),
  ).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("2 of 2 done.")).toBeVisible();
  await page.getByRole("button", { name: /docs-site/ }).click();
  await expect(page.getByRole("log", { name: "Output" })).toContainText("Restarting docs-site");
  await expect(page.getByRole("button", { name: "Retry failed" })).toHaveCount(0);
  expect(errors()).toEqual([]);
});

test("a rebuild with a failure lists it and retries only the failed app (PRJ-5, JOB-5)", async ({ page }) => {
  await signIn(page);
  await page.goto("/projects/internal-tools");
  await page.getByRole("button", { name: "More actions for Internal tools" }).click();
  await page.getByRole("menuitem", { name: "Rebuild all" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Rebuild all" }).click();
  await openJob(page);
  await expect(page.getByText("1 of 2 failed.")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("status-page");

  await page.getByRole("button", { name: "Retry failed" }).click();
  await expect(page.getByText("1 of 1 failed.")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("button", { name: /status-page/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /docs-site/ })).toHaveCount(0);
});
