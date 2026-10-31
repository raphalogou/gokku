import { expect, test } from "@playwright/test";
import { signIn, watchErrors } from "./helpers";

// Runs after account.spec.ts. Leaves blog-api with extra variables and the Blog platform
// project with shared variables, which later specs do not look at.
test.describe.configure({ mode: "serial" });

test("edit app config, with masked and managed variables (CFG-1 to CFG-4)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps/blog-api/config");
  const list = page.getByRole("list", { name: "Environment variables" });
  await expect(list.getByText("Set by link to blog-db")).toBeVisible();
  await expect(page.getByLabel("Value DATABASE_URL")).toHaveAttribute("readonly", "");
  await expect(page.getByLabel("Value SESSION_SECRET")).toHaveValue("");

  // CFG-2: reveal a secret.
  await page.getByRole("button", { name: "Show SESSION_SECRET" }).click();
  await expect(page.getByLabel("Value SESSION_SECRET")).toHaveValue("fake-session-secret-0123456789");

  await page.getByLabel("Value NODE_ENV").fill("staging");
  await page.getByLabel("New key").fill("FEATURE_FLAG");
  await page.getByLabel("Value", { exact: true }).fill("on");
  await page.getByRole("button", { name: "Add variable" }).click();
  await page.getByRole("button", { name: "Remove TZ" }).click();
  await expect(page.getByText("3 unsaved changes")).toBeVisible();
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText("Config saved.")).toBeVisible();

  await page.reload();
  await expect(page.getByLabel("Value NODE_ENV")).toHaveValue("staging");
  await expect(page.getByLabel("Value FEATURE_FLAG")).toHaveValue("on");
  await expect(page.getByLabel("Value TZ")).toHaveCount(0);
  expect(errors()).toEqual([]);
});

test("edit as .env, then save with a restart (CFG-4, CFG-5)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/blog-api/config");
  await page.getByRole("button", { name: "Edit as .env" }).click();
  const dialog = page.getByRole("dialog", { name: "Edit as .env" });
  const text = dialog.getByLabel("Variables");
  await text.fill(
    `${await text.inputValue()}\nCACHE_TTL=300`.replace("NODE_ENV=staging", "NODE_ENV=production"),
  );
  await expect(dialog.getByText("1 added, 1 changed, 0 removed")).toBeVisible();
  await dialog.getByRole("button", { name: "Apply to the form" }).click();
  await expect(page.getByText("2 unsaved changes")).toBeVisible();
  await page.getByLabel("Restart app after saving").check();
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page).toHaveURL(/\/jobs\//);
  await expect(page.getByRole("heading", { level: 1, name: "Restart blog-api" })).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
});

test("shared variables apply to the project's apps (SHV-1 to SHV-7)", async ({ page }) => {
  await signIn(page);
  await page.goto("/projects/blog-platform/config");
  await page.getByLabel("New key").fill("LOG_LEVEL");
  await page.getByLabel("Value", { exact: true }).fill("info");
  await page.getByRole("button", { name: "Add variable" }).click();
  await page.getByLabel("New key").fill("DATABASE_URL");
  await page.getByLabel("Value", { exact: true }).fill("x");
  await page.getByRole("button", { name: "Add variable" }).click();
  await page.getByRole("button", { name: "Save and apply" }).click();
  // SHV-4: a key a datastore sets on link is reserved.
  await expect(page.getByText("Some variables cannot be saved.")).toBeVisible();
  await page.getByRole("button", { name: "Remove DATABASE_URL" }).click();
  await page.getByRole("button", { name: "Save and apply" }).click();

  await expect(page).toHaveURL(/\/jobs\//);
  await expect(
    page.getByRole("heading", { level: 1, name: "Apply shared variables to Blog platform" }),
  ).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("blog-api: set LOG_LEVEL");

  await page.goto("/projects/blog-platform/config");
  await expect(
    page.getByRole("table", { name: "Shared variables" }).getByText("Applied to 2 apps"),
  ).toBeVisible();
  await page.goto("/apps/worker-queue/config");
  await expect(page.getByText("Inherited from project Blog platform")).toBeVisible();
  await expect(page.getByLabel("Value LOG_LEVEL")).toHaveAttribute("readonly", "");
});

test("an operator can edit app config but not shared variables", async ({ page }) => {
  await signIn(page, "olga", "olga's long password");
  await page.goto("/apps/storefront/config");
  await expect(page.getByRole("button", { name: "Edit as .env" })).toBeVisible();
  await page.goto("/projects/storefront/config");
  await expect(page.getByText("Only admins can change shared variables.")).toBeVisible();
  await expect(page.getByLabel("New key")).toHaveCount(0);
});
