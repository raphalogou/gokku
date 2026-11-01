import { expect, test } from "@playwright/test";
import { openJob, signIn, watchErrors } from "./helpers";

// Runs after scaling.spec.ts. The service it creates is destroyed at the end.
test.describe.configure({ mode: "serial" });

test("create a service, link, reveal, unlink and destroy it (SVC-2 to SVC-4)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/services");
  await page.getByRole("link", { name: "Create service" }).click();
  const dialog = page.getByRole("dialog", { name: "Create service" });
  await dialog.getByLabel("Type").click();
  await page.getByRole("option", { name: "Redis" }).click();
  await dialog.getByLabel("Name").fill("jobs-cache");
  await dialog.getByLabel("Project").click();
  await page.getByRole("option", { name: "Internal tools" }).click();
  await dialog.getByRole("button", { name: "Create service" }).click();
  await openJob(page);
  await expect(
    page.getByRole("heading", { level: 1, name: "Create redis service jobs-cache" }),
  ).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("container created");
  await expect(page.getByRole("log", { name: "Output" })).not.toContainText("fake-password");

  await page.goto("/services");
  await page.getByRole("table", { name: "All services" }).getByRole("link", { name: "jobs-cache" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "jobs-cache" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Internal tools" })).toBeVisible();

  // Connection: masked, then revealed (SVC-4).
  const url = page.getByLabel("Value connection URL");
  await expect(url).toHaveAttribute("type", "password");
  await page.getByRole("button", { name: "Show connection URL" }).click();
  await expect(url).toHaveValue(/^redis:\/\/redis:fake-password@dokku-redis-jobs-cache/);
  await page.getByRole("button", { name: "Hide connection URL" }).click();

  // Link an app of the same project (SVC-3).
  await page.getByLabel("Link another app").click();
  await page.getByRole("option", { name: "status-page" }).click();
  await page.getByRole("button", { name: "Link app" }).click();
  await openJob(page);
  await expect(
    page.getByRole("heading", { level: 1, name: "Link jobs-cache and status-page" }),
  ).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("REDIS_URL");
  await expect(page.getByRole("log", { name: "Output" })).not.toContainText("fake-password");

  await page.goto("/services/redis/jobs-cache");
  const links = page.getByRole("table", { name: "Linked apps" });
  await expect(links.getByRole("link", { name: "status-page" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Destroy service" })).toBeDisabled();
  await page.getByRole("button", { name: "Unlink status-page" }).click();
  await openJob(page);
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });

  await page.goto("/services/redis/jobs-cache");
  await expect(page.getByText("No app is linked.")).toBeVisible();
  await page.getByRole("button", { name: "Destroy service" }).click();
  const confirm = page.getByRole("dialog", { name: "Destroy jobs-cache?" });
  await confirm.getByLabel("Type jobs-cache to confirm").fill("jobs-cache");
  await confirm.getByRole("button", { name: "Destroy service" }).click();
  await openJob(page);
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await page.goto("/services");
  await expect(
    page.getByRole("table", { name: "All services" }).getByRole("link", { name: "jobs-cache" }),
  ).toHaveCount(0);
  expect(errors()).toEqual([]);
});

test("stop asks first, then start brings the service back (SVC-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/services/redis/blog-cache");
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await page.getByRole("dialog", { name: "Stop blog-cache?" }).getByRole("button", { name: "Stop" }).click();
  await openJob(page);
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await page.goto("/services/redis/blog-cache");
  await expect(page.getByText("Stopped", { exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await openJob(page);
  await expect(page.getByText("Succeeded", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await page.goto("/services/redis/blog-cache");
  await expect(page.getByText("Running", { exact: true }).first()).toBeVisible();
});

test("an operator can link but not create or destroy", async ({ page }) => {
  await signIn(page, "olga", "olga's long password");
  await page.goto("/services");
  await expect(page.getByRole("link", { name: "Create service" })).toHaveCount(0);
  await page.goto("/services/postgres/shop-db");
  await expect(page.getByRole("button", { name: "Restart" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Destroy service" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Show connection URL" })).toBeVisible();
});

test("services are stacked cards on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page);
  await page.goto("/services");
  await expect(page.getByRole("link", { name: "blog-db" })).toBeVisible();
  await expect(page.getByRole("table", { name: "All services" })).toBeHidden();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});
