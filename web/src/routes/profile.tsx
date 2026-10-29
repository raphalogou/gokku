import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Monitor } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Banner } from "~/components/app/banner";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { PageHeader } from "~/components/app/page-header";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { changePassword, revokeMySession, updateMe } from "~/lib/api/gen/client";
import type { Session } from "~/lib/api/gen/model";
import { dateTime, describeUserAgent, timeAgo } from "~/lib/format";
import { applyServerError, errorMessage } from "~/lib/forms";
import { meQuery, mySessionsQuery, queryKeys, setupStatusQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { newPasswordSchema, username } from "~/lib/schemas";
import { strings } from "~/lib/strings";

export async function clientLoader() {
  await Promise.all([
    queryClient.ensureQueryData(mySessionsQuery()),
    queryClient.ensureQueryData(setupStatusQuery()),
  ]);
  return null;
}

export default function Profile() {
  return (
    <>
      <PageHeader title={strings.profile.title} description={strings.profile.description} />
      <div className="flex max-w-3xl flex-col gap-6">
        <UsernameCard />
        <PasswordCard />
        <SessionsCard />
      </div>
    </>
  );
}

/** USR-1: rename yourself and stay signed in. */
function UsernameCard() {
  const { data: me } = useSuspenseQuery(meQuery());
  const qc = useQueryClient();
  const schema = z.object({ username });
  const form = useForm({ resolver: zodResolver(schema), values: { username: me.username } });
  const { errors, isSubmitting, isDirty } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const updated = await updateMe({ username: values.username });
      qc.setQueryData(queryKeys.me(), updated);
      notify(strings.profile.usernameSaved);
    } catch (err) {
      applyServerError(err, form.setError, ["username"]);
    }
  });

  return (
    <Card>
      <CardHeader title={strings.profile.accountTitle} />
      <CardBody>
        <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4 sm:flex-row sm:items-start">
          <Field
            label={strings.profile.username}
            hint={strings.profile.usernameHint}
            error={errors.username?.message ?? errors.root?.message}
            className="flex-1"
          >
            <Input
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              {...form.register("username")}
            />
          </Field>
          <Button type="submit" disabled={!isDirty || isSubmitting} className="sm:mt-[27px]">
            {isSubmitting ? strings.common.saving : strings.common.save}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}

/** USR-2: change your password; other sessions are signed out. */
function PasswordCard() {
  const { data: status } = useSuspenseQuery(setupStatusQuery());
  const qc = useQueryClient();
  const schema = z
    .object({ current: z.string().min(1, strings.validation.required) })
    .and(newPasswordSchema(status.min_password_length));
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: { current: "", password: "", confirm: "" },
  });
  const { errors, isSubmitting } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await changePassword({ current_password: values.current, new_password: values.password });
      form.reset();
      await qc.invalidateQueries({ queryKey: queryKeys.mySessions() });
      notify(strings.profile.passwordChanged);
    } catch (err) {
      applyServerError(err, form.setError, { current_password: "current", new_password: "password" });
    }
  });

  return (
    <Card>
      <CardHeader title={strings.profile.passwordTitle} description={strings.profile.passwordIntro} />
      <CardBody>
        <form onSubmit={onSubmit} noValidate className="flex max-w-md flex-col gap-5">
          {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
          <Field label={strings.profile.currentPassword} error={errors.current?.message}>
            <Input type="password" autoComplete="current-password" {...form.register("current")} />
          </Field>
          <Field
            label={strings.profile.newPassword}
            hint={strings.setup.passwordHint(status.min_password_length)}
            error={errors.password?.message}
          >
            <Input type="password" autoComplete="new-password" {...form.register("password")} />
          </Field>
          <Field label={strings.profile.confirmPassword} error={errors.confirm?.message}>
            <Input type="password" autoComplete="new-password" {...form.register("confirm")} />
          </Field>
          <div>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? strings.common.saving : strings.profile.changePassword}
            </Button>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

/** USR-3: list your sessions and sign out the others. */
function SessionsCard() {
  const { data: sessions } = useSuspenseQuery(mySessionsQuery());
  const qc = useQueryClient();
  const revoke = useMutation({
    mutationFn: (s: Session) => revokeMySession(s.id),
    onSuccess: () => {
      notify(strings.profile.sessionRevoked);
      return qc.invalidateQueries({ queryKey: queryKeys.mySessions() });
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  return (
    <Card>
      <CardHeader title={strings.profile.sessionsTitle} description={strings.profile.sessionsIntro} />
      <ul className="divide-y divide-line">
        {sessions.map((s) => (
          <li key={s.id} className="flex min-h-[60px] flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3">
            <Monitor className="size-[18px] text-muted" aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="font-semibold">
                {describeUserAgent(s.user_agent)}
                {s.current ? (
                  <span className="ml-2 rounded-full bg-nav-active px-2 py-0.5 text-[13px] text-accent">
                    {strings.profile.thisSession}
                  </span>
                ) : null}
              </div>
              <div className="text-[13px] text-muted">
                <span className="font-mono">{s.ip || "—"}</span> ·{" "}
                <time dateTime={s.last_seen_at} title={dateTime(s.last_seen_at)}>
                  {strings.profile.lastActive(timeAgo(s.last_seen_at))}
                </time>
              </div>
            </div>
            {s.current ? null : (
              <Button variant="secondary" onClick={() => revoke.mutate(s)} disabled={revoke.isPending}>
                {strings.profile.signOutSession}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </Card>
  );
}
