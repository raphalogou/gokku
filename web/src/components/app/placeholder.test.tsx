import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Placeholder } from "./placeholder";

test("shows the product name as the page heading", () => {
  render(<Placeholder />);
  expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Gokku");
});
