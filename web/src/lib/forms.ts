import type { FieldValues, Path, UseFormSetError } from "react-hook-form";
import { ApiError } from "./api/fetcher";
import { strings } from "./strings";

/**
 * Maps an API error onto a form. Field errors go to the matching inputs; `fields` lists the
 * form fields that share the API's names, or maps API names to form fields. Anything else
 * goes to the form's root error, which the form shows in a banner.
 */
export function applyServerError<T extends FieldValues>(
  err: unknown,
  setError: UseFormSetError<T>,
  fields: Path<T>[] | Partial<Record<string, Path<T>>>,
) {
  if (!(err instanceof ApiError)) {
    setError("root", { message: strings.errors.unexpected });
    return;
  }
  const map: Partial<Record<string, Path<T>>> = Array.isArray(fields)
    ? Object.fromEntries(fields.map((f) => [f, f]))
    : fields;
  let mapped = false;
  for (const [name, message] of Object.entries(err.fields)) {
    const target = map[name];
    if (target) {
      setError(target, { message });
      mapped = true;
    }
  }
  if (!mapped) setError("root", { message: err.message });
}

export function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : strings.errors.unexpected;
}
