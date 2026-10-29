import { expect, test } from "@playwright/test";
import { watchErrors } from "./helpers";

test("health endpoint answers", async ({ request }) => {
  const res = await request.get("/healthz");
  expect(res.ok()).toBe(true);
  expect(await res.json()).toMatchObject({ status: "ok" });
});

test("app loads under the CSP without errors", async ({ page }) => {
  const errors = watchErrors(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  // The inline theme script ran, so CSP hashes match the built index.html.
  await expect(page.locator("html")).toHaveAttribute("data-theme", /^(light|dark)$/);
  expect(errors()).toEqual([]);
});

test("client routes fall back to the app", async ({ page }) => {
  const res = await page.goto("/apps/some-app");
  expect(res?.status()).toBe(200);
  expect(res?.headers()["content-type"]).toContain("text/html");
});

test("stored theme preference applies before the app renders", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("gokku-theme", "dark"));
  await page.goto("/login");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});
