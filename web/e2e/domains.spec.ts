import { expect, test } from "@playwright/test";
import { openJob, signIn, watchErrors } from "./helpers";

// Runs after config.spec.ts; leaves docs-site with an extra domain.
test.describe.configure({ mode: "serial" });

test("add and remove a domain, with the DNS reminder (DOM-1, SSL-3)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/apps/docs-site/domains");
  await expect(page.getByText(/Point an A record for each domain to this server/)).toBeVisible();
  const table = page.getByRole("table", { name: "Domains" });
  await expect(table.getByRole("row", { name: /docs\.example\.com.*No certificate/ })).toBeVisible();

  await page.getByLabel("Add a domain").fill("not a domain");
  await page.getByRole("button", { name: "Add domain" }).click();
  await expect(page.getByText("Use a host name such as app.example.com.")).toBeVisible();
  await page.getByLabel("Add a domain").fill("Help.Example.com");
  await page.getByRole("button", { name: "Add domain" }).click();
  await expect(page.getByText("help.example.com was added.")).toBeVisible();
  await expect(table.getByRole("row", { name: /help\.example\.com/ })).toBeVisible();

  await page.getByRole("button", { name: "Remove help.example.com" }).click();
  await expect(page.getByText("help.example.com was removed.")).toBeVisible();
  await expect(table.getByRole("row", { name: /help\.example\.com/ })).toHaveCount(0);
  expect(errors()).toEqual([]);
});

test("enable SSL as a job and see the certificate (SSL-1, SSL-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/docs-site/domains");
  await page.getByLabel("Contact email").fill("docs@example.com");
  await page.getByRole("button", { name: "Save email" }).click();
  await expect(page.getByText("Contact email saved.")).toBeVisible();

  await page.getByRole("button", { name: "Enable SSL" }).click();
  await openJob(page, "Enable SSL for docs-site");
  await expect(page.getByRole("heading", { level: 1, name: "Enable SSL for docs-site" })).toBeVisible();
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });

  await page.goto("/apps/docs-site/domains");
  await expect(
    page
      .getByRole("table", { name: "Domains" })
      .getByRole("row", { name: /docs\.example\.com.*Active, expires/ }),
  ).toBeVisible();

  // A domain added after the certificate is flagged until a renewal.
  await page.getByLabel("Add a domain").fill("www.docs.example.com");
  await page.getByRole("button", { name: "Add domain" }).click();
  await expect(
    page.getByText("www.docs.example.com was added after the certificate was issued.", { exact: false }),
  ).toBeVisible({
    timeout: 10_000,
  });
  await expect(page.getByRole("button", { name: "Renew now" })).toBeVisible();
});
