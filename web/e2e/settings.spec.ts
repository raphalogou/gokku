import { expect, test } from "@playwright/test";
import { signIn, watchErrors } from "./helpers";

// Runs after services.spec.ts and leaves every setting as it found it.
test.describe.configure({ mode: "serial" });

test("global domains are added and removed in Dokku (SET-1)", async ({ page }) => {
  const errors = watchErrors(page);
  await signIn(page);
  await page.goto("/settings");
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
  const domains = page.getByRole("list", { name: "Domain" });
  await expect(domains).toContainText("dokku-01.example.com");
  await page.getByLabel("Add a global domain").fill("apps.example.com");
  await page.getByRole("button", { name: "Add domain" }).click();
  await expect(domains).toContainText("apps.example.com");
  await page.reload();
  await expect(domains).toContainText("apps.example.com");
  await page.getByRole("button", { name: "Remove apps.example.com" }).click();
  await expect(domains).not.toContainText("apps.example.com");
  await expect(page.getByText("Gokku version")).toBeVisible();
  expect(errors()).toEqual([]);

  // A bad name is refused by the server and shown on the field.
  await page.getByLabel("Add a global domain").fill("not a domain");
  await page.getByRole("button", { name: "Add domain" }).click();
  await expect(page.getByText("not a domain is not a domain name.")).toBeVisible();
});

test("a protected app refuses stop until it is unprotected (SET-2)", async ({ page }) => {
  await signIn(page);
  await page.goto("/settings");
  await page.getByLabel("Protect an app").click();
  await page.getByRole("option", { name: "staging-web" }).click();
  await page.getByRole("button", { name: "Protect", exact: true }).click();
  const list = page.getByRole("list", { name: "Protected apps" });
  await expect(list).toContainText("staging-web");

  await page.goto("/apps/staging-web/settings");
  await page.getByRole("button", { name: "Destroy app" }).click();
  const confirm = page.getByRole("dialog", { name: "Destroy staging-web?" });
  await confirm.getByLabel("Type staging-web to confirm").fill("staging-web");
  await confirm.getByRole("button", { name: "Destroy app" }).click();
  await expect(confirm.getByText(/protected/i)).toBeVisible();
  await confirm.getByRole("button", { name: "Cancel" }).click();

  await page.goto("/settings");
  await page.getByRole("button", { name: "Stop protecting staging-web" }).click();
  await expect(page.getByText("No app is protected.")).toBeVisible();
});

test("security settings save and validate (SET-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/settings");
  const save = page.getByRole("button", { name: "Save" });
  await expect(save).toBeDisabled();
  await page.getByLabel(/Keep job output for/).fill("0");
  await save.click();
  await expect(page.getByText("Use a number from 1 to 3650.")).toBeVisible();
  await page.getByLabel(/Keep job output for/).fill("14");
  await page.getByLabel("Session lifetime").click();
  await page.getByRole("option", { name: "7 days" }).click();
  await save.click();
  await expect(page.getByText("Settings saved.")).toBeVisible();
  await page.reload();
  await expect(page.getByLabel(/Keep job output for/)).toHaveValue("14");

  // Put them back.
  await page.getByLabel(/Keep job output for/).fill("30");
  await page.getByLabel("Session lifetime").click();
  await page.getByRole("option", { name: "1 day" }).click();
  await save.click();
  await expect(page.getByText("Settings saved.").first()).toBeVisible();
});

test("settings are for admins only (SET-1)", async ({ page }) => {
  await signIn(page, "olga", "olga's long password");
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Not available for your role" })).toBeVisible();
});
