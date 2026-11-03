import { expect, test } from "@playwright/test";
import { admin, openAccountMenu, signIn, watchErrors } from "./helpers";

test.describe.configure({ mode: "serial" });

test("first run: setup creates the admin and signs in (AUTH-1)", async ({ page }) => {
  const errors = watchErrors(page);
  await page.goto("/");
  await expect(page).toHaveURL(/\/setup$/);
  await expect(page.getByRole("heading", { name: "Create the first admin" })).toBeVisible();

  await page.getByLabel("Username").fill(admin.username);
  await page.getByLabel("Password", { exact: true }).fill("short");
  await page.getByLabel("Repeat password").fill("short");
  await page.getByRole("button", { name: "Create admin and sign in" }).click();
  await expect(page.getByText("Use at least 12 characters.")).toBeVisible();

  await page.getByLabel("Password", { exact: true }).fill(admin.password);
  await page.getByLabel("Repeat password").fill(admin.password);
  await page.getByRole("button", { name: "Create admin and sign in" }).click();

  await expect(page).toHaveURL(/\/projects$/);
  await expect(page.getByRole("button", { name: "Account menu" })).toContainText("alice");
  expect(errors()).toEqual([]);

  // Setup is now closed.
  await page.goto("/setup");
  await expect(page).not.toHaveURL(/\/setup$/);
});

test("sign out, wrong password, and sign in back to the requested page", async ({ page }) => {
  await signIn(page);
  await openAccountMenu(page);
  await page.getByRole("menuitem", { name: "Log out" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.goto("/profile");
  await expect(page).toHaveURL(/\/login\?next=%2Fprofile$/);
  await page.getByLabel("Username").fill(admin.username);
  await page.getByLabel("Password").fill("not the password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toContainText("Wrong username or password.");

  await page.getByLabel("Password").fill(admin.password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Profile" })).toBeVisible();
});

test("theme choice survives a reload and a new browser (USR-4)", async ({ page, browser }) => {
  await signIn(page);
  await openAccountMenu(page);
  const dark = page.getByRole("button", { name: "Dark" });
  // Wait for the save: the reload below must not overtake it.
  const saved = page.waitForResponse(
    (r) => r.url().endsWith("/api/v1/me") && r.request().method() === "PATCH",
  );
  await dark.click();
  await saved;
  await expect(dark).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  const fresh = await browser.newContext({ colorScheme: "light" });
  const other = await fresh.newPage();
  await signIn(other);
  await expect(other.locator("html")).toHaveAttribute("data-theme", "dark");
  await fresh.close();

  // Back to light for the remaining tests.
  await page.keyboard.press("Escape");
  await openAccountMenu(page);
  await page.getByRole("button", { name: "Light" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});

test("renaming yourself keeps the session (USR-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/profile");
  const name = page.getByLabel("Username");
  await name.fill("alice-ops");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Username changed.")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("button", { name: "Account menu" })).toContainText("alice-ops");

  await name.fill(admin.username);
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Username changed.")).toBeVisible();
});

test("invite an operator, who signs in without admin screens (AUTH-7)", async ({ page, browser }) => {
  await signIn(page);
  await page.getByRole("link", { name: "Users" }).click();
  await page.getByRole("button", { name: "Invite user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Username").fill("olga");
  await dialog.getByLabel("Role").click();
  await page.getByRole("option", { name: /Operator/ }).click();
  await dialog.getByRole("button", { name: "Create invite link" }).click();

  const link =
    (await page.getByRole("dialog", { name: "Invite link created" }).locator("code").textContent()) ?? "";
  expect(link).toMatch(/\/invite\/[\w-]+$/);
  await page.getByRole("button", { name: "Done" }).click();
  await expect(page.getByRole("row", { name: /olga/ })).toContainText("Invited");

  const ctx = await browser.newContext();
  const olga = await ctx.newPage();
  const errors = watchErrors(olga);
  await olga.goto(link);
  await expect(olga.getByLabel("Username")).toHaveValue("olga");
  await olga.getByLabel("Password", { exact: true }).fill("olga's long password");
  await olga.getByLabel("Repeat password").fill("olga's long password");
  await olga.getByRole("button", { name: "Set password and sign in" }).click();
  await expect(olga.getByRole("heading", { level: 1, name: "Projects" })).toBeVisible();
  await expect(olga.getByRole("link", { name: "Users" })).toHaveCount(0);

  await olga.goto("/users");
  await expect(olga.getByRole("heading", { name: "Not available for your role" })).toBeVisible();
  expect(errors().filter((e) => !e.includes("403"))).toEqual([]);
  await ctx.close();

  await page.reload();
  await expect(page.getByRole("row", { name: /olga/ })).toContainText("Active");
});

test("create and revoke an API token (USR-6)", async ({ page }) => {
  await signIn(page);
  await page.goto("/users");
  await page.getByRole("button", { name: "Create token" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name").fill("ci-deploy");
  await dialog.getByRole("button", { name: "Create token" }).click();

  const secret =
    (await page.getByRole("dialog", { name: "Token created" }).locator("code").textContent()) ?? "";
  expect(secret).toMatch(/^gk_[0-9A-Za-z]{40}$/);
  await page.getByRole("button", { name: "Done" }).click();

  const res = await page.request.get("/api/v1/me", { headers: { Authorization: `Bearer ${secret}` } });
  expect(res.ok()).toBe(true);

  const row = page.getByRole("row", { name: /ci-deploy/ });
  await row.getByRole("button", { name: "Revoke" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Revoke" }).click();
  await expect(page.getByText("ci-deploy was revoked.")).toBeVisible();
  const after = await page.request.get("/api/v1/me", { headers: { Authorization: `Bearer ${secret}` } });
  expect(after.status()).toBe(401);
});

test("phone layout has a bottom tab bar and no horizontal scroll", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await ctx.newPage();
  await signIn(page);
  await expect(page.getByRole("button", { name: "Account menu" })).toBeHidden();
  const tabs = page.getByRole("navigation", { name: "Main" });
  await expect(tabs.getByRole("link", { name: "Apps" })).toBeVisible();

  await tabs.getByRole("button", { name: "More" }).click();
  await page.getByRole("dialog").getByRole("link", { name: "Users" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Users" })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBe(0);
  await ctx.close();
});
