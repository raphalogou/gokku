import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { type ReactNode, useState } from "react";
import { data } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { PageHeader } from "~/components/app/page-header";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { Select } from "~/components/ui/select";
import { notify } from "~/components/ui/toast";
import { ApiError } from "~/lib/api/fetcher";
import { updateSettings } from "~/lib/api/gen/client";
import type { Settings as SettingsData, UpdateSettingsRequest } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { appsQuery, meQuery, queryKeys, serverQuery, settingsQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";

export async function clientLoader() {
  const me = await queryClient.ensureQueryData(meQuery());
  if (me.role !== "admin") throw data(null, { status: 403 });
  void queryClient.prefetchQuery(appsQuery());
  await queryClient.ensureQueryData(settingsQuery());
  return null;
}

/** Settings (SET-1, SET-2): global domains, protected apps, server information, security
 * and job retention. Admins only. */
export default function Settings() {
  const { data: settings } = useSuspenseQuery(settingsQuery());
  const t = strings.settings;
  return (
    <>
      <PageHeader title={t.title} description={t.description} />
      <div className="flex max-w-[860px] flex-col gap-6">
        <GlobalDomains settings={settings} />
        <ProtectedApps settings={settings} />
        <ServerInfo settings={settings} />
        <Security settings={settings} />
      </div>
    </>
  );
}

function useSave() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: UpdateSettingsRequest) => updateSettings(req),
    onSuccess: (res) => {
      qc.setQueryData(queryKeys.settings(), res);
      notify(strings.settings.saved);
    },
  });
}

/** A list with remove buttons and an add row, as Global domains in the mockup. */
function ListCard({
  title,
  hint,
  column,
  items,
  empty,
  removeLabel,
  onRemove,
  pending,
  error,
  add,
}: {
  title: string;
  hint: string;
  column: string;
  items: string[];
  empty: string;
  removeLabel: (item: string) => string;
  onRemove: (item: string) => void;
  pending: boolean;
  error?: string;
  add: ReactNode;
}) {
  return (
    <Card>
      <CardHeader title={title} />
      <p className="px-5 pt-4 text-sm text-muted">{hint}</p>
      {error ? (
        <div className="px-5 pt-4">
          <Banner variant="error">{error}</Banner>
        </div>
      ) : null}
      {items.length === 0 ? (
        <p className="px-5 py-5 text-muted">{empty}</p>
      ) : (
        <ul aria-label={column} className="mt-3 border-t border-line">
          {items.map((item) => (
            <li
              key={item}
              className="flex min-h-[60px] items-center justify-between gap-4 border-b border-line px-5 py-2 last:border-b-0"
            >
              <span className="min-w-0 font-mono text-sm break-all">{item}</span>
              <Button
                variant="ghost"
                size="icon"
                aria-label={removeLabel(item)}
                onClick={() => onRemove(item)}
                disabled={pending}
              >
                <Trash2 aria-hidden />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="border-t border-line bg-subtle/40 px-5 py-4">{add}</div>
    </Card>
  );
}

function GlobalDomains({ settings }: { settings: SettingsData }) {
  const t = strings.settings;
  const save = useSave();
  const [domain, setDomain] = useState("");
  const [fieldError, setFieldError] = useState<string | undefined>();
  if (!settings.global_domains) {
    return (
      <Card>
        <CardHeader title={t.globalDomains} />
        <CardBody>
          <p className="text-muted">{t.domainsUnavailable}</p>
        </CardBody>
      </Card>
    );
  }
  const domains = settings.global_domains;
  const submit = (next: string[], onDone?: () => void) => {
    setFieldError(undefined);
    save.mutate(
      { global_domains: next },
      {
        onSuccess: onDone,
        onError: (err) => setFieldError(err instanceof ApiError ? err.fields.global_domains : undefined),
      },
    );
  };
  return (
    <ListCard
      title={t.globalDomains}
      hint={t.globalDomainsHint}
      column={t.domain}
      items={domains}
      empty={t.noGlobalDomains}
      removeLabel={t.removeDomain}
      onRemove={(d) => submit(domains.filter((x) => x !== d))}
      pending={save.isPending}
      error={save.error && !fieldError ? errorMessage(save.error) : undefined}
      add={
        <form
          noValidate
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const d = domain.trim().toLowerCase();
            if (d) submit([...domains, d], () => setDomain(""));
          }}
        >
          <Field label={t.addGlobal} error={fieldError} className="flex-[1_1_240px]">
            <Input
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.org"
              autoCapitalize="none"
              spellCheck={false}
              className="font-mono"
            />
          </Field>
          <Button type="submit" variant="secondary" disabled={save.isPending || !domain.trim()}>
            <Plus aria-hidden />
            {t.addDomain}
          </Button>
        </form>
      }
    />
  );
}

function ProtectedApps({ settings }: { settings: SettingsData }) {
  const t = strings.settings;
  const save = useSave();
  const apps = useQuery(appsQuery());
  const [app, setApp] = useState("");
  const candidates = (apps.data ?? []).map((a) => a.name).filter((n) => !settings.protected_apps.includes(n));
  const chosen = app && candidates.includes(app) ? app : (candidates[0] ?? "");
  return (
    <ListCard
      title={t.protectedApps}
      hint={t.protectedHint}
      column={t.protectedApps}
      items={settings.protected_apps}
      empty={t.noProtected}
      removeLabel={t.unprotect}
      onRemove={(a) => save.mutate({ protected_apps: settings.protected_apps.filter((x) => x !== a) })}
      pending={save.isPending}
      error={save.error ? errorMessage(save.error) : undefined}
      add={
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (chosen) save.mutate({ protected_apps: [...settings.protected_apps, chosen] });
          }}
        >
          <Field label={t.addProtected} className="flex-[1_1_240px]">
            <Select
              value={chosen}
              onValueChange={setApp}
              options={candidates.map((n) => ({ value: n, label: n }))}
            />
          </Field>
          <Button type="submit" variant="secondary" disabled={save.isPending || !chosen}>
            <Plus aria-hidden />
            {t.protect}
          </Button>
        </form>
      }
    />
  );
}

function ServerInfo({ settings }: { settings: SettingsData }) {
  const t = strings.settings;
  const { data: server } = useQuery(serverQuery());
  const items: [string, string][] = [
    [t.serverName, server?.name ?? "—"],
    [t.dokkuHost, server?.host ?? "—"],
    [t.dokkuVersion, server?.dokku_version ?? "—"],
    [t.gokkuVersion, settings.gokku_version || "—"],
  ];
  return (
    <Card>
      <CardHeader title={t.server} />
      <CardBody>
        <dl className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          {items.map(([label, value]) => (
            <div key={label} className="flex min-w-0 flex-col gap-0.5">
              <dt className="text-[13px] font-semibold text-muted">{label}</dt>
              <dd className="font-mono text-sm break-all">{value}</dd>
            </div>
          ))}
        </dl>
      </CardBody>
    </Card>
  );
}

function Security({ settings }: { settings: SettingsData }) {
  const t = strings.settings;
  const save = useSave();
  const [lifetime, setLifetime] = useState(String(settings.session_lifetime_hours));
  const [minLength, setMinLength] = useState(String(settings.min_password_length));
  const [retention, setRetention] = useState(String(settings.job_retention_days));
  const [fields, setFields] = useState<Record<string, string>>({});
  const lifetimes = { ...t.lifetimes };
  if (!(settings.session_lifetime_hours in lifetimes)) {
    lifetimes[settings.session_lifetime_hours] = t.hours(settings.session_lifetime_hours);
  }
  const dirty =
    lifetime !== String(settings.session_lifetime_hours) ||
    minLength !== String(settings.min_password_length) ||
    retention !== String(settings.job_retention_days);

  return (
    <Card>
      <CardHeader title={t.security} />
      <CardBody>
        <form
          noValidate
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault();
            setFields({});
            save.mutate(
              {
                session_lifetime_hours: Number(lifetime),
                min_password_length: Number(minLength),
                job_retention_days: Number(retention),
              },
              {
                onError: (err) =>
                  setFields(
                    err instanceof ApiError && Object.keys(err.fields).length
                      ? err.fields
                      : { root: errorMessage(err) },
                  ),
              },
            );
          }}
        >
          {fields.root ? <Banner variant="error">{fields.root}</Banner> : null}
          <Field label={t.sessionLifetime} hint={t.sessionLifetimeHint} error={fields.session_lifetime_hours}>
            <Select
              value={lifetime}
              onValueChange={setLifetime}
              options={Object.entries(lifetimes)
                .sort(([a], [b]) => Number(a) - Number(b))
                .map(([value, label]) => ({ value, label }))}
              className="w-full sm:w-[200px]"
            />
          </Field>
          <Field label={t.passwordLength} hint={t.passwordLengthHint} error={fields.min_password_length}>
            <Input
              type="number"
              inputMode="numeric"
              min={8}
              max={128}
              value={minLength}
              onChange={(e) => setMinLength(e.target.value)}
              className="w-full sm:w-[200px]"
            />
          </Field>
          <Field
            label={`${t.retention} (${t.days})`}
            hint={t.retentionHint}
            error={fields.job_retention_days}
          >
            <Input
              type="number"
              inputMode="numeric"
              min={1}
              max={3650}
              value={retention}
              onChange={(e) => setRetention(e.target.value)}
              className="w-full sm:w-[200px]"
            />
          </Field>
          <div>
            <Button type="submit" disabled={!dirty || save.isPending}>
              {save.isPending ? t.saving : t.save}
            </Button>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}
