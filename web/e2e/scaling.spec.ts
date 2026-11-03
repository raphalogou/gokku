import { expect, test } from "@playwright/test";
import { openJob, signIn } from "./helpers";

test("scale a process type and apply as a job (SCL-1)", async ({ page }) => {
  await signIn(page);
  await page.goto("/apps/storefront/scaling");
  const table = page.getByRole("table", { name: "Process types" });
  await expect(table.getByRole("row", { name: /web.*Running.*No change/ })).toBeVisible();
  await page.getByRole("button", { name: "Increase web instances" }).click();
  await expect(table.getByText("Changed from 1")).toBeVisible();
  await expect(page.getByText("1 unsaved change")).toBeVisible();
  await page.getByRole("button", { name: "Apply and restart" }).click();
  await openJob(page, "Scale storefront");
  await expect(page.getByText("Succeeded", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log", { name: "Output" })).toContainText("web=2");

  await page.goto("/apps/storefront/scaling");
  await expect(page.getByRole("spinbutton", { name: "web instances" })).toHaveValue("2");
});
