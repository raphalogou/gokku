import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Field } from "./field";
import { Input } from "./input";

test("labels the control and links the error for screen readers", () => {
  render(
    <Field label="Username" hint="Lowercase only" error="This username is taken.">
      <Input />
    </Field>,
  );
  const input = screen.getByLabelText("Username");
  expect(input.getAttribute("aria-invalid")).toBe("true");
  const describedBy = input.getAttribute("aria-describedby") ?? "";
  expect(document.getElementById(describedBy.split(" ")[0] ?? "")?.textContent).toBe(
    "This username is taken.",
  );
});

test("shows the hint when there is no error", () => {
  render(
    <Field label="Username" hint="Lowercase only">
      <Input />
    </Field>,
  );
  const input = screen.getByLabelText("Username");
  expect(input.getAttribute("aria-invalid")).toBeNull();
  expect(document.getElementById(input.getAttribute("aria-describedby") ?? "")?.textContent).toBe(
    "Lowercase only",
  );
});
