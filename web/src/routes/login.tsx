import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { redirect, useNavigate, useSearchParams } from "react-router";
import { z } from "zod";
import { Banner } from "~/components/app/banner";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { login } from "~/lib/api/gen/client";
import { applyServerError } from "~/lib/forms";
import { safeNext } from "~/lib/navigation";
import { queryKeys, setupStatusQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import { applyTheme } from "~/lib/theme";
import { PublicCard } from "./public-layout";

export async function clientLoader() {
  const status = await queryClient.fetchQuery(setupStatusQuery());
  if (status.setup_required) throw redirect("/setup");
  return null;
}

const schema = z.object({
  username: z.string().trim().min(1, strings.validation.required),
  password: z.string().min(1, strings.validation.required),
});
type Values = z.infer<typeof schema>;

export default function Login() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", password: "" },
  });
  const { errors, isSubmitting } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const me = await login(values);
      qc.setQueryData(queryKeys.me(), me);
      applyTheme(me.theme); // the account's theme wins after sign-in (USR-4)
      navigate(safeNext(params.get("next")), { replace: true });
    } catch (err) {
      form.resetField("password");
      applyServerError(err, form.setError, []);
    }
  });

  return (
    <PublicCard title={strings.login.title}>
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
        <Field label={strings.login.username} error={errors.username?.message}>
          <Input
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            autoFocus
            {...form.register("username")}
          />
        </Field>
        <Field label={strings.login.password} error={errors.password?.message}>
          <Input type="password" autoComplete="current-password" {...form.register("password")} />
        </Field>
        <Button type="submit" disabled={isSubmitting} className="mt-1 w-full">
          {isSubmitting ? strings.login.submitting : strings.login.submit}
        </Button>
      </form>
    </PublicCard>
  );
}
