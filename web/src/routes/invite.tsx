import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Button, buttonVariants } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { ApiError } from "~/lib/api/fetcher";
import { acceptInvite, getInvite } from "~/lib/api/gen/client";
import { applyServerError } from "~/lib/forms";
import { queryKeys, setupStatusQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { newPasswordSchema } from "~/lib/schemas";
import { strings } from "~/lib/strings";
import { applyTheme } from "~/lib/theme";
import type { Route } from "./+types/invite";
import { PublicCard } from "./public-layout";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  const status = await queryClient.fetchQuery(setupStatusQuery());
  try {
    const invite = await getInvite(params.token);
    return { invite, minPasswordLength: status.min_password_length, error: null };
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      return { invite: null, minPasswordLength: status.min_password_length, error: err.message };
    }
    throw err;
  }
}

/** Accept an invite (AUTH-7). Reuses the setup layout with the username read-only. */
export default function AcceptInvite({ loaderData, params }: Route.ComponentProps) {
  const { invite, minPasswordLength: min, error } = loaderData;
  const schema = newPasswordSchema(min);
  const qc = useQueryClient();
  const navigate = useNavigate();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { password: "", confirm: "" } });
  const { errors, isSubmitting } = form.formState;

  if (!invite) {
    return (
      <PublicCard title={strings.invite.invalidTitle}>
        <p className="text-[15px] text-muted">{error}</p>
        <Link to="/login" className={buttonVariants({ variant: "secondary", className: "mt-6 w-full" })}>
          {strings.invite.goToSignIn}
        </Link>
      </PublicCard>
    );
  }

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const me = await acceptInvite({ token: params.token, password: values.password });
      qc.setQueryData(queryKeys.me(), me);
      applyTheme(me.theme);
      navigate("/", { replace: true });
    } catch (err) {
      applyServerError(err, form.setError, ["password"]);
    }
  });

  return (
    <PublicCard title={strings.invite.title} intro={strings.invite.intro}>
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
        <Field label={strings.invite.username}>
          <Input value={invite.username} readOnly autoComplete="username" />
        </Field>
        <Field
          label={strings.setup.password}
          hint={strings.setup.passwordHint(min)}
          error={errors.password?.message}
        >
          <Input type="password" autoComplete="new-password" autoFocus {...form.register("password")} />
        </Field>
        <Field label={strings.setup.confirm} error={errors.confirm?.message}>
          <Input type="password" autoComplete="new-password" {...form.register("confirm")} />
        </Field>
        <Button type="submit" disabled={isSubmitting} className="mt-1 w-full">
          {isSubmitting ? strings.setup.submitting : strings.invite.submit}
        </Button>
      </form>
    </PublicCard>
  );
}
