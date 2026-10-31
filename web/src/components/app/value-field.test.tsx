import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ValueField } from "./value-field";

const base = { name: "API_TOKEN", onChange: () => {}, readOnly: false };

describe("ValueField", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hides a secret behind a password input, with reveal and copy inside the field", () => {
    const onReveal = vi.fn();
    render(
      <ValueField {...base} value="" hidden secret onReveal={onReveal} readHidden={async () => "tok"} />,
    );
    expect(screen.getByLabelText("Value API_TOKEN").getAttribute("type")).toBe("password");
    fireEvent.click(screen.getByRole("button", { name: "Show API_TOKEN" }));
    expect(onReveal).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Copy API_TOKEN" })).toBeTruthy();
  });

  it("offers neither reveal nor copy of a secret to a role that cannot reveal it (CFG-2)", () => {
    render(<ValueField {...base} value="" hidden secret />);
    expect(screen.queryByRole("button", { name: "Show API_TOKEN" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy API_TOKEN" })).toBeNull();
  });

  it("copies a hidden value through readHidden without showing it", async () => {
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    const readHidden = vi.fn(async () => "s3cret");
    render(<ValueField {...base} value="" hidden secret onReveal={() => {}} readHidden={readHidden} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy API_TOKEN" }));
    });
    expect(readHidden).toHaveBeenCalled();
    expect(writeText).toHaveBeenCalledWith("s3cret");
    expect((screen.getByLabelText("Value API_TOKEN") as HTMLInputElement).value).toBe("");
    expect(screen.getByText("Copied")).toBeTruthy();
  });

  it("copies a plain value directly and offers no reveal", async () => {
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    render(<ValueField {...base} name="NODE_ENV" value="production" hidden={false} secret={false} />);
    expect(screen.queryByRole("button", { name: /Show|Hide/ })).toBeNull();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy NODE_ENV" }));
    });
    expect(writeText).toHaveBeenCalledWith("production");
  });
});
