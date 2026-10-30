import { describe, expect, it } from "vitest";
import { projectSchema, slugify } from "./schemas";

describe("slugify", () => {
  it.each([
    ["Blog platform", "blog-platform"],
    ["  Internal tools!! ", "internal-tools"],
    ["Café & Bar", "cafe-bar"],
    ["---", ""],
    [`${"a".repeat(39)} b`, "a".repeat(39)],
  ])("%s becomes %s", (name, slug) => {
    expect(slugify(name)).toBe(slug);
  });
});

describe("projectSchema", () => {
  it("accepts a project the API accepts (PRJ-1)", () => {
    expect(projectSchema.safeParse({ name: "Blog", slug: "blog-platform", description: "" }).success).toBe(
      true,
    );
  });

  it.each([
    [{ name: "", slug: "blog", description: "" }, "name"],
    [{ name: "Blog", slug: "Blog Platform", description: "" }, "slug"],
    [{ name: "Blog", slug: "-blog", description: "" }, "slug"],
    [{ name: "Blog", slug: "blog", description: "x".repeat(501) }, "description"],
  ])("rejects %o on %s", (value, field) => {
    const res = projectSchema.safeParse(value);
    expect(res.success).toBe(false);
    expect(res.error?.issues[0]?.path).toEqual([field]);
  });
});
