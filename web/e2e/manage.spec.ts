import { expect, type Page, test } from "@playwright/test";
import { openJob, signIn, watchErrors } from "./helpers";

// Runs before projects.spec.ts, so every test restores what it changes: the new app is
// destroyed, the renamed app gets its name back and the moved app returns.
test.describe.configure({ mode: "serial" });

async function waitForJob(page: Page, title: string) {
  await openJob(page, title);
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
}

test("create an app in a project, then destroy it (APP-3, APP-5)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps");
  await page.getByRole("link", { name: "New app" }).click();
  const dialog = page.getByRole("dialog", { name: "New app" });
  await dialog.getByLabel("App name").fill("Bad_Name");
  await dialog.getByRole("button", { name: "Create app" }).click();
  await expect(dialog.getByText(/Use lowercase letters, numbers, dots and dashes/)).toBeVisible();
  await dialog.getByLabel("App name").fill("landing");
  await dialog.getByRole("radio", { name: /Push with git/ }).check();
  await dialog.getByRole("button", { name: "Create app" }).click();

  const done = page.getByRole("dialog", { name: "landing was created" });
  await expect(done.getByText(/git remote add dokku dokku@127\.0\.0\.1:landing/)).toBeVisible();
  await done.getByRole("link", { name: "Open app" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "landing" })).toBeVisible();

  await page.getByRole("navigation", { name: "landing" }).getByRole("link", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Destroy app" }).click();
  const confirm = page.getByRole("dialog", { name: "Destroy landing?" });
  await confirm.getByLabel("Type landing to confirm").fill("landing");
  await confirm.getByRole("button", { name: "Destroy app" }).click();
  await waitForJob(page, "Destroy landing");
  await page.goto("/apps");
  await expect(page.getByText("6 apps on dokku-01")).toBeVisible({ timeout: 10_000 });
  expect(errors()).toEqual([]);
});

test("rename an app and move it between projects (APP-4, APP-7)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site/settings");
  await page.getByLabel("App name", { exact: true }).fill("handbook");
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await waitForJob(page, "Rename docs-site to handbook");
  await page.goto("/apps/handbook/settings");
  await expect(page.getByRole("heading", { level: 1, name: "handbook" })).toBeVisible({ timeout: 10_000 });

  await page.getByLabel("App name", { exact: true }).fill("docs-site");
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await waitForJob(page, "Rename handbook to docs-site");

  await page.goto("/apps/staging-web/settings");
  await page.getByLabel("Project").click();
  await page.getByRole("option", { name: "Internal tools" }).click();
  await page.getByRole("button", { name: "Move" }).click();
  await expect(page.getByText("Moved to Internal tools.")).toBeVisible();
  await expect(page.getByText("Internal tools").first()).toBeVisible();
  await page.getByLabel("Project").click();
  await page.getByRole("option", { name: "Storefront" }).click();
  await page.getByRole("button", { name: "Move" }).click();
  await expect(page.getByText("Moved to Storefront.")).toBeVisible();
});

test("edit and delete a project (PRJ-1, PRJ-4)", async ({ page }) => {
  await signIn(page);
  await page.goto("/projects/new");
  await page.getByRole("dialog").getByLabel("Name", { exact: true }).fill("Scratch");
  await page.getByRole("dialog").getByRole("button", { name: "Create project" }).click();
  await expect(page).toHaveURL(/\/projects\/scratch$/);

  await page.getByRole("navigation", { name: "Scratch" }).getByRole("link", { name: "Settings" }).click();
  await expect(page.getByLabel("URL name")).toHaveAttribute("readonly", "");
  await page.getByLabel("Description").fill("Temporary");
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText("Project saved.")).toBeVisible();
  await expect(page.getByText("Temporary").first()).toBeVisible();

  await page.getByRole("button", { name: "Delete project" }).click();
  const confirm = page.getByRole("dialog", { name: "Delete Scratch?" });
  await confirm.getByLabel("Type scratch to confirm").fill("scratch");
  await confirm.getByRole("button", { name: "Delete project" }).click();
  await expect(page).toHaveURL(/\/projects$/);
  await expect(page.getByText("Scratch was deleted.")).toBeVisible();
});

test("create an app from its project, which is chosen already (APP-3)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/projects/storefront");
  await page.getByRole("link", { name: "Create app" }).click();
  await expect(page).toHaveURL(/\/projects\/storefront\/apps\/new$/);
  const dialog = page.getByRole("dialog", { name: "New app" });
  await expect(dialog.getByLabel("Project")).toHaveText(/Storefront/);
  await dialog.getByLabel("App name").fill("shop-admin");
  await dialog.getByRole("radio", { name: /Docker image/ }).check();
  await dialog.getByRole("button", { name: "Create app" }).click();
  await page
    .getByRole("dialog", { name: "shop-admin was created" })
    .getByRole("link", { name: "Open app" })
    .click();
  await expect(page.getByRole("link", { name: "Storefront" }).first()).toBeVisible();

  // Closing without creating returns to the project.
  await page.goto("/projects/storefront/apps/new");
  await page.getByRole("dialog", { name: "New app" }).getByRole("button", { name: "Cancel" }).click();
  await expect(page).toHaveURL(/\/projects\/storefront\/apps$/);
  await expect(
    page.getByRole("table", { name: "Apps in this project" }).getByRole("link", { name: "shop-admin" }),
  ).toBeVisible();

  await page.goto("/apps/shop-admin/settings");
  await page.getByRole("button", { name: "Destroy app" }).click();
  const confirm = page.getByRole("dialog", { name: "Destroy shop-admin?" });
  await confirm.getByLabel("Type shop-admin to confirm").fill("shop-admin");
  await confirm.getByRole("button", { name: "Destroy app" }).click();
  await waitForJob(page, "Destroy shop-admin");
  expect(errors()).toEqual([]);
});
