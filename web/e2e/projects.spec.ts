import { rm, writeFile } from "node:fs/promises";
import { expect, type Page, test } from "@playwright/test";
import { signIn, watchErrors } from "./helpers";

// The fake runner serves the mockup's apps and services, and the server seeds the mockup's
// three projects on first start (NFR-8). These tests run after account.spec.ts.
test.describe.configure({ mode: "serial" });

const failFile = "e2e/.data/fake-fail";

async function pick(page: Page, label: string, option: RegExp) {
  await page.getByRole("dialog").getByLabel(label, { exact: true }).click();
  await page.getByRole("option", { name: option }).click();
}

test("projects show their apps and the worst status (PRJ-3, APP-2)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  const blog = page
    .getByRole("listitem")
    .filter({ has: page.getByRole("heading", { name: "Blog platform" }) });
  await expect(blog.getByText("1 deploying")).toBeVisible();
  await expect(blog.getByText("blog-db postgres")).toBeVisible();
  const tools = page
    .getByRole("listitem")
    .filter({ has: page.getByRole("heading", { name: "Internal tools" }) });
  await expect(tools.getByText("1 failed")).toBeVisible();
  await expect(tools.getByText("No services")).toBeVisible();

  await page.getByRole("link", { name: "Blog platform" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Blog platform" })).toBeVisible();
  const apps = page.getByRole("table", { name: "Apps" });
  await expect(apps.getByRole("row", { name: /blog-api.*Running.*blog-api\.example\.com/ })).toBeVisible();
  await expect(apps.getByRole("row", { name: /worker-queue.*Deploying.*No domain/ })).toBeVisible();
  const services = page.getByRole("table", { name: "Services" });
  await expect(
    services.getByRole("row", { name: /blog-cache.*Redis.*blog-api.*worker-queue/ }),
  ).toBeVisible();
  expect(errors()).toEqual([]);
});

test("create a project, then move an app and a service into it (PRJ-1, PRJ-2)", async ({ page }) => {
  await signIn(page);
  await page.getByRole("link", { name: "New project" }).click();
  const dialog = page.getByRole("dialog", { name: "New project" });
  await dialog.getByLabel("Name", { exact: true }).fill("Data pipeline");
  await expect(dialog.getByLabel("URL name")).toHaveValue("data-pipeline");
  await dialog.getByLabel("Description (optional)").fill("Nightly jobs and exports");
  await dialog.getByRole("button", { name: "Create project" }).click();

  await expect(page).toHaveURL(/\/projects\/data-pipeline$/);
  await expect(page.getByText("No apps yet")).toBeVisible();
  await expect(page.getByText("No services yet")).toBeVisible();

  await page.getByRole("button", { name: "Add app" }).first().click();
  await pick(page, "App", /docs-site.*in Internal tools/);
  await page.getByRole("dialog").getByRole("button", { name: "Add to project" }).click();
  await expect(page.getByText("docs-site was added to the project.")).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Apps" }).getByRole("link", { name: "docs-site" }),
  ).toBeVisible();

  await page.getByRole("button", { name: "Add service" }).first().click();
  await pick(page, "Service", /shop-db/);
  await page.getByRole("dialog").getByRole("button", { name: "Add to project" }).click();
  await expect(page.getByRole("table", { name: "Services" }).getByText("shop-db")).toBeVisible();

  // The app left its old project: an app belongs to one project.
  await page.goto("/projects/internal-tools");
  await expect(
    page.getByRole("table", { name: "Apps" }).getByRole("link", { name: "docs-site" }),
  ).toHaveCount(0);

  // Release it again from the Apps tab.
  await page.goto("/projects/data-pipeline/apps");
  await page.getByRole("button", { name: "More actions for docs-site" }).click();
  await page.getByRole("menuitem", { name: "Remove from project" }).click();
  await expect(page.getByText("docs-site was removed from the project.")).toBeVisible();
  await expect(page.getByText("No apps yet")).toBeVisible();
});

test("apps list filters by project and opens an app (APP-1, PRJ-6)", async ({ page }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Apps" }).click();
  await expect(page.getByText("6 apps on dokku-01")).toBeVisible();
  const table = page.getByRole("table", { name: "All apps" });
  await expect(table.getByRole("row", { name: /status-page.*Failed/ })).toBeVisible();
  // docs-site was released by the previous test and is unassigned (PRJ-6).
  await expect(table.getByRole("row", { name: /docs-site.*Unassigned/ })).toBeVisible();

  await page.getByLabel("Filter by project").click();
  await page.getByRole("option", { name: "Storefront" }).click();
  await expect(table.getByRole("row")).toHaveCount(3); // header, storefront, staging-web
  await page.getByLabel("Search apps").fill("staging");
  await expect(table.getByRole("row")).toHaveCount(2);

  await table.getByRole("link", { name: "staging-web" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "staging-web" })).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Processes" }).getByRole("row", { name: /web.*1.*Stopped/ }),
  ).toBeVisible();
  await expect(page.getByText("shop-db")).toBeVisible();
});

test("services and plugins come from the capability probe (SVC-1, PLG-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/services");
  const table = page.getByRole("table", { name: "All services" });
  await expect(table.getByRole("row")).toHaveCount(4);
  await page.getByRole("button", { name: "Redis 1" }).click();
  await expect(table.getByRole("row")).toHaveCount(2);
  await expect(page.getByText(/MySQL.*are not installed on this server/)).toBeVisible();

  await page.goto("/plugins");
  const installed = page.getByRole("table", { name: "Installed" });
  await expect(installed.getByRole("row", { name: /postgres.*Postgres services.*Installed/ })).toBeVisible();
  await expect(installed.getByRole("row", { name: /letsencrypt/ })).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Available, not installed" }).getByRole("row", { name: /mysql/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Rescan" }).click();
  await expect(page.getByText("Plugins rescanned.")).toBeVisible();
});

test("an unreachable Dokku shows diagnostics and the last known state (SRV-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps");
  await writeFile(failFile, "sudo: a password is required");
  try {
    await page.getByRole("link", { name: "Plugins" }).click();
    await page.getByRole("button", { name: "Rescan" }).click();
    const alert = page.getByRole("alert").filter({ hasText: "Cannot reach Dokku on dokku-01" });
    await expect(alert).toBeVisible();
    await expect(alert.getByText("$ dokku version")).toBeVisible();
    await expect(alert.getByText("sudo: a password is required")).toBeVisible();
    await expect(page.getByText("Unreachable")).toBeVisible();

    await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Apps" }).click();
    await expect(page.getByRole("table", { name: "All apps" }).getByRole("row")).toHaveCount(7);
  } finally {
    await rm(failFile, { force: true });
  }
  await page.getByRole("button", { name: "Retry" }).click();
  await expect(page.getByText("Cannot reach Dokku")).toHaveCount(0);
  await expect(page.getByText("Connected")).toBeVisible();
});

test("an operator sees projects without admin controls", async ({ page }) => {
  await signIn(page, "olga", "olga's long password");
  await expect(page.getByRole("link", { name: "New project" })).toHaveCount(0);
  await page.getByRole("link", { name: "Blog platform" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Blog platform" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add app" })).toHaveCount(0);
  await page.goto("/plugins");
  await expect(page.getByRole("button", { name: "Rescan" })).toHaveCount(0);
});

test("phone layouts stack and do not scroll sideways", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await ctx.newPage();
  await signIn(page);
  for (const path of [
    "/projects",
    "/projects/blog-platform",
    "/apps",
    "/apps/blog-api",
    "/services",
    "/plugins",
  ]) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow, path).toBe(0);
  }
  await ctx.close();
});
