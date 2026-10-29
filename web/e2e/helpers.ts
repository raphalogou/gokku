import { expect, type Page } from "@playwright/test";

export const admin = { username: "alice", password: "correct horse battery" };

/**
 * Collects console errors and uncaught exceptions, so a test can assert that a page ran
 * cleanly under the Content-Security-Policy. A 401 from probing /api/v1/me while signed out
 * is expected and ignored.
 */
export function watchErrors(page: Page): () => string[] {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error" && !msg.text().includes("401 (Unauthorized)")) errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push(err.message));
  return () => errors;
}

export async function signIn(page: Page, username = admin.username, password = admin.password) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Projects" })).toBeVisible();
}

export async function openAccountMenu(page: Page) {
  await page.getByRole("button", { name: "Account menu" }).click();
}
