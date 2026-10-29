import type { Theme } from "./api/gen/model";

// The pre-paint script in root.tsx reads the same key (DESIGN.md, 5.3).
const storageKey = "gokku-theme";

export function storedTheme(): Theme {
  try {
    const v = localStorage.getItem(storageKey);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    // Storage can be unavailable (private mode, blocked site data).
  }
  return "system";
}

export function resolveTheme(pref: Theme): "light" | "dark" {
  if (pref !== "system") return pref;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Applies a theme preference to the page and caches it for the next first paint. */
export function applyTheme(pref: Theme) {
  document.documentElement.dataset.theme = resolveTheme(pref);
  try {
    localStorage.setItem(storageKey, pref);
  } catch {
    // See storedTheme.
  }
}
