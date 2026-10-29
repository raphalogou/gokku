import { z } from "zod";
import { InviteUserBody } from "./api/gen/zod";
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
