import { expect, test } from "@playwright/test";
import { openJob, signIn, watchErrors } from "./helpers";

// Runs after config.spec.ts. The failed deploy comes first so docs-site ends running.
test.describe.configure({ mode: "serial" });

test("the deploy tab shows how to push with git (DEP-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site/deploy");
  await expect(page.getByText(/^git remote add dokku dokku@.+:docs-site$/)).toBeVisible();
  await expect(page.getByText(/^git push dokku \S+$/)).toBeVisible();
});

test("a repository deploy that fails shows the failed step (DEP-1, DEP-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site/deploy");
  await page.getByLabel("Repository URL").fill("https://github.com/acme/docs.git");
  await page.getByLabel("Branch or tag").fill("broken-build");
  await page.getByRole("button", { name: "Deploy repository" }).click();
  await openJob(page, "Deploy https://github.com/acme/docs.git at broken-build");
  await expect(page).toHaveURL(/\/apps\/docs-site\/deploys\/[0-9A-Z]+$/);
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("Build failed");
  await expect(page.getByRole("listitem").filter({ hasText: "Build image" })).toContainText("Failed after");
  await expect(page.getByRole("listitem").filter({ hasText: "Release" })).toContainText("Skipped");
});

test("validation errors show on the field (DEP-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site/deploy");
  await page.getByLabel("Repository URL").fill("file:///etc");
  await page.getByRole("button", { name: "Deploy repository" }).click();
  await expect(page.getByText(/remote repository, without a password/)).toBeVisible();
});

test("deploy an image, follow its steps and see it in the history (DEP-1 to DEP-3)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps/docs-site/deploy");
  await page.getByLabel("Image", { exact: true }).fill("nginx:1.28-alpine");
  await page.getByRole("button", { name: "Deploy image" }).click();
  await openJob(page, "Deploy nginx:1.28-alpine");
  await expect(page.getByRole("heading", { level: 1, name: "Deploy nginx:1.28-alpine" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "Fetch source" })).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("Application deployed");
  await expect(page.getByRole("listitem").filter({ hasText: "Release" })).toContainText("Done in");

  await page.goto("/apps/docs-site/deploy");
  const history = page.getByRole("table", { name: "Deployment history" });
  await expect(
    history.getByRole("row", { name: /Docker image.*nginx:1\.28-alpine.*Succeeded/ }),
  ).toBeVisible();
  await expect(history.getByRole("row", { name: /Git repository.*broken-build.*Failed/ })).toBeVisible();
  await history.getByRole("link", { name: "View output" }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Deploy nginx:1.28-alpine" })).toBeVisible();
  expect(errors()).toEqual([]);
});
