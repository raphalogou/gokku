import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { redirect, useNavigate } from "react-router";
import { z } from "zod";
import { Banner } from "~/components/app/banner";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { completeSetup } from "~/lib/api/gen/client";
import { applyServerError } from "~/lib/forms";
import { queryKeys, setupStatusQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { newPasswordSchema, username } from "~/lib/schemas";
import { strings } from "~/lib/strings";
import { PublicCard } from "./public-layout";

export async function clientLoader() {
  const status = await queryClient.fetchQuery(setupStatusQuery());
  if (!status.setup_required) throw redirect("/login");
  return null;
}

/** First-run setup: creates the first admin (AUTH-1). */
export default function Setup() {
  const { data: status } = useSuspenseQuery(setupStatusQuery());
  const min = status.min_password_length;
  const schema = z.object({ username }).and(newPasswordSchema(min));
  type Values = z.infer<typeof schema>;

  const qc = useQueryClient();
  const navigate = useNavigate();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", password: "", confirm: "" },
  });
  const { errors, isSubmitting } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const me = await completeSetup({ username: values.username, password: values.password });
      qc.setQueryData(queryKeys.me(), me);
      qc.setQueryData(queryKeys.setupStatus(), { ...status, setup_required: false });
      navigate("/", { replace: true });
    } catch (err) {
      applyServerError(err, form.setError, ["username", "password"]);
    }
  });

  return (
    <PublicCard title={strings.setup.title} intro={strings.setup.intro}>
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
        <Field
          label={strings.setup.username}
          hint={strings.setup.usernameHint}
          error={errors.username?.message}
        >
          <Input
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            autoFocus
            {...form.register("username")}
          />
        </Field>
        <Field
          label={strings.setup.password}
          hint={strings.setup.passwordHint(min)}
          error={errors.password?.message}
        >
          <Input type="password" autoComplete="new-password" {...form.register("password")} />
        </Field>
        <Field label={strings.setup.confirm} error={errors.confirm?.message}>
          <Input type="password" autoComplete="new-password" {...form.register("confirm")} />
        </Field>
        <Button type="submit" disabled={isSubmitting} className="mt-1 w-full">
          {isSubmitting ? strings.setup.submitting : strings.setup.submit}
        </Button>
      </form>
    </PublicCard>
  );
}
