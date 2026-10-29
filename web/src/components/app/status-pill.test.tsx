import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { StatusPill } from "./status-pill";

test("always shows the status as text, not only as color (NFR-4)", () => {
  render(<StatusPill kind="failed" label="Failed" />);
  expect(screen.getByText("Failed")).toBeTruthy();
});
