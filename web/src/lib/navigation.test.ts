import { expect, test } from "vitest";
import { safeNext } from "./navigation";

test("keeps same-site paths", () => {
  expect(safeNext("/users")).toBe("/users");
  expect(safeNext("/apps/web?tab=logs")).toBe("/apps/web?tab=logs");
});

test("rejects anything that could leave the site", () => {
  for (const bad of [
    null,
    undefined,
    "",
    "https://evil.example",
    "//evil.example",
    "/\\evil.example",
    "javascript:alert(1)",
  ]) {
    expect(safeNext(bad)).toBe("/");
  }
});
