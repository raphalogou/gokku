import { describe, expect, it } from "vitest";
import { diffDotenv, formatDotenv, parseDotenv } from "./dotenv";

describe("parseDotenv (CFG-5)", () => {
  it("reads the usual forms", () => {
    const { vars, errors } = parseDotenv(
      [
        "# comment",
        "",
        "A=1",
        "export B = two",
        'C="quoted \\"x\\"\\nline"',
        "D='single # kept'",
        "E=bare # dropped",
        "F=",
      ].join("\n"),
    );
    expect(errors).toEqual([]);
    expect(vars).toEqual({ A: "1", B: "two", C: 'quoted "x"\nline', D: "single # kept", E: "bare", F: "" });
  });

  it("reports bad lines", () => {
    expect(parseDotenv("=1\nnot a pair\n1A=x\nOK=1").errors).toEqual([
      "Line 1: expected KEY=value",
      "Line 2: expected KEY=value",
      "Line 3: expected KEY=value",
    ]);
  });

  it("round-trips through formatDotenv", () => {
    const vars = { A: "plain", B: 'needs "quotes" and spaces', C: "two\nlines" };
    expect(parseDotenv(formatDotenv(vars)).vars).toEqual(vars);
  });
});

describe("diffDotenv", () => {
  it("lists added, changed and removed keys, keeping read-only ones", () => {
    const diff = diffDotenv(
      { A: "1", B: "2", C: "3" },
      { A: "1", B: "two", D: "4", SECRET: "new" },
      new Set(["SECRET", "MANAGED"]),
    );
    expect(diff).toEqual({ added: ["D"], changed: ["B", "SECRET"], removed: ["C"] });
  });
});
