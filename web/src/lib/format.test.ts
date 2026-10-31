import { describe, expect, it, test } from "vitest";
import { describeUserAgent, duration, timeAgo } from "./format";

const now = Date.parse("2026-10-29T12:00:00Z");

test("timeAgo", () => {
  expect(timeAgo("2026-10-29T11:59:30Z", now)).toBe("just now");
  expect(timeAgo("2026-10-29T11:55:00Z", now)).toBe("5 minutes ago");
  expect(timeAgo("2026-10-28T12:00:00Z", now)).toBe("yesterday");
  expect(timeAgo("2026-11-05T12:00:00Z", now)).toBe("next week");
});

test("describeUserAgent", () => {
  expect(describeUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0")).toBe(
    "Firefox on Linux",
  );
  expect(
    describeUserAgent(
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0 Safari/537.36",
    ),
  ).toBe("Chrome on macOS");
  expect(describeUserAgent("curl/8.9.1")).toBe("curl");
  expect(describeUserAgent("")).toBe("Unknown device");
});

describe("duration", () => {
  it.each([
    ["2026-10-30T09:00:00Z", "2026-10-30T09:00:02Z", "2s"],
    ["2026-10-30T09:00:00Z", "2026-10-30T09:01:38Z", "1m 38s"],
    ["2026-10-30T09:00:00Z", "2026-10-30T11:05:00Z", "2h 5m"],
    ["2026-10-30T09:00:05Z", "2026-10-30T09:00:00Z", "0s"],
  ])("%s to %s is %s", (from, to, want) => {
    expect(duration(from, to)).toBe(want);
  });
});
