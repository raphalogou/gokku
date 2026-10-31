import { describe, expect, it } from "vitest";
import { appendCapped, type ConsoleLine, consoleText, detectLevel, maxConsoleLines } from "./console";

describe("detectLevel (LOG-5)", () => {
  it.each([
    ["WARN slow query", "stdout", "WARN", "slow query"],
    ["[error] boom", "stdout", "ERROR", "boom"],
    ["INFO: started", "stdout", "INFO", "started"],
    ["POST /comments 500: ERROR validation", "stdout", "ERROR", "POST /comments 500: ERROR validation"],
    ["GET /health 200", "stdout", "INFO", "GET /health 200"],
    ["something on stderr", "stderr", "WARN", "something on stderr"],
    ["information about errors", "stdout", "INFO", "information about errors"],
  ])("%s", (text, stream, level, rest) => {
    expect(detectLevel(text, stream)).toEqual({ level, text: rest });
  });
});

describe("console buffer", () => {
  const line = (i: number): ConsoleLine => ({ key: String(i), level: "INFO", text: `line ${i}` });

  it("keeps the newest lines only", () => {
    const many = Array.from({ length: maxConsoleLines + 10 }, (_, i) => line(i));
    const kept = appendCapped([], many);
    expect(kept).toHaveLength(maxConsoleLines);
    expect(kept[0]?.key).toBe("10");
  });

  it("downloads the visible text", () => {
    expect(consoleText([{ key: "1", time: "09:14:02", label: "web.1", level: "WARN", text: "slow" }])).toBe(
      "09:14:02  web.1  WARN  slow\n",
    );
  });
});
