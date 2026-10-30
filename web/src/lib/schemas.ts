import { z } from "zod";
import { CreateProjectBody, InviteUserBody } from "./api/gen/zod";
import { strings } from "./strings";

/** Username rules from the API spec (USR-1), with the UI's message. */
export const username = z
  .string()
  .trim()
  .refine((v) => InviteUserBody.shape.username.safeParse(v).success, strings.validation.username);

/** A new password with confirmation; the minimum length is a server setting (AUTH-2). */
export function newPasswordSchema(min: number) {
  return z
    .object({
      password: z.string().min(min, strings.validation.passwordMin(min)),
      confirm: z.string(),
    })
    .refine((v) => v.password === v.confirm, {
      path: ["confirm"],
      message: strings.validation.passwordsDiffer,
    });
}

/** A new project (PRJ-1): the generated limits with the UI's messages. */
export const projectSchema = z.object({
  name: z
    .string()
    .trim()
    .refine((v) => CreateProjectBody.shape.name.safeParse(v).success, strings.validation.projectName),
  slug: z
    .string()
    .trim()
    .refine((v) => CreateProjectBody.shape.slug.safeParse(v).success, strings.validation.projectSlug),
  description: z
    .string()
    .trim()
    .refine(
      (v) => CreateProjectBody.shape.description.safeParse(v).success,
      strings.validation.projectDescription,
    ),
});

/** Turns a project name into a URL name: "Blog platform" becomes "blog-platform". */
export function slugify(name: string): string {
  return name
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/, "");
}
