import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Lock, Plus, RefreshCw, ShieldOff, Trash2 } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Link, useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { Skeleton } from "~/components/app/states";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { ApiError } from "~/lib/api/fetcher";
import { addAppDomain, changeAppSSL, removeAppDomain } from "~/lib/api/gen/client";
import type { SSLRequest, SSLStatus } from "~/lib/api/gen/model";
import { dateTime, timeAgo } from "~/lib/format";
import { errorMessage } from "~/lib/forms";
import { appDomainsQuery, appSSLQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-domains";

const domainRe = /^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

/** Domains and SSL (DOM-1, SSL-1 to SSL-3). */
export default function AppDomains({ params }: Route.ComponentProps) {
  const name = params.name;
  const domains = useQuery(appDomainsQuery(name));
  const ssl = useQuery(appSSLQuery(name));
  const canWrite = useCan("app.domains.write");
  const qc = useQueryClient();
  const t = strings.domains;

  const remove = useMutation({
    mutationFn: (d: string) => removeAppDomain(name, { domain: d }),
    onSuccess: (data, d) => {
      qc.setQueryData(queryKeys.appDomains(name), data);
      void qc.invalidateQueries({ queryKey: queryKeys.appSSL(name) });
      notify(t.removed(d));
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const sslFor = (d: string): string => {
    const s = ssl.data;
    if (!s?.active) return t.noCert;
    if (s.uncovered.includes(d)) return t.notCovered;
    return t.active(s.expires_at ? timeAgo(s.expires_at) : "");
  };

  return (
    <div className="flex max-w-4xl flex-col gap-6">
      {domains.data ? <Banner>{t.dns(domains.data.dns_target)}</Banner> : null}
      <Card>
        <CardHeader title={t.title} />
        {domains.isPending ? (
          <div className="p-5">
            <Skeleton className="h-24" />
          </div>
        ) : domains.isError ? (
          <div className="p-5">
            <Banner variant="error">{errorMessage(domains.error)}</Banner>
          </div>
        ) : domains.data.domains.length === 0 ? (
          <p className="px-5 py-6 text-muted">{t.empty}</p>
        ) : (
          <DataTable label={t.title} className="min-w-[520px]">
            <THead>
              <TH>{t.domain}</TH>
              <TH>{t.ssl}</TH>
              {canWrite ? (
                <TH className="w-16">
                  <span className="sr-only">{strings.users.columns.actions}</span>
                </TH>
              ) : null}
            </THead>
            <tbody>
              {domains.data.domains.map((d) => (
                <TR key={d}>
                  <TD className="font-medium break-all">{d}</TD>
                  <TD className={ssl.data?.uncovered.includes(d) ? "text-deploying-fg" : "text-muted"}>
                    {sslFor(d)}
                  </TD>
                  {canWrite ? (
                    <TD className="py-0 text-right">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t.remove(d)}
                        onClick={() => remove.mutate(d)}
                        disabled={remove.isPending}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    </TD>
                  ) : null}
                </TR>
              ))}
            </tbody>
          </DataTable>
        )}
        {canWrite ? <AddDomain name={name} existing={domains.data?.domains ?? []} /> : null}
      </Card>
      <SSLCard
        name={name}
        ssl={ssl.data}
        loading={ssl.isPending}
        error={ssl.error}
        hasDomains={(domains.data?.domains.length ?? 0) > 0}
      />
    </div>
  );
}

function AddDomain({ name, existing }: { name: string; existing: string[] }) {
  const t = strings.domains;
  const qc = useQueryClient();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const errorId = useId();
  const add = useMutation({
    mutationFn: (domain: string) => addAppDomain(name, { domain }),
    onSuccess: (data, d) => {
      qc.setQueryData(queryKeys.appDomains(name), data);
      void qc.invalidateQueries({ queryKey: queryKeys.appSSL(name) });
      setValue("");
      notify(t.added(d));
    },
    onError: (err) =>
      setError(err instanceof ApiError ? (err.fields.domain ?? err.message) : errorMessage(err)),
  });
  return (
    <form
      className="flex flex-wrap items-start gap-3 border-t border-line bg-add-row-bg px-5 py-3"
      onSubmit={(e) => {
        e.preventDefault();
        const d = value.trim().toLowerCase();
        if (!domainRe.test(d)) return setError(t.invalid);
        if (existing.includes(d)) return setError(strings.config.keyTaken);
        setError(null);
        add.mutate(d);
      }}
    >
      <div className="min-w-0 flex-[1_1_260px]">
        <Input
          aria-label={t.newDomain}
          placeholder="app.example.com"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : undefined}
          autoCapitalize="none"
          spellCheck={false}
        />
        {error ? (
          <p id={errorId} className="mt-1 text-sm text-failed">
            {error}
          </p>
        ) : null}
      </div>
      <Button type="submit" variant="secondary" disabled={!value.trim() || add.isPending}>
        <Plus aria-hidden />
        {add.isPending ? t.adding : t.add}
      </Button>
    </form>
  );
}

function SSLCard({
  name,
  ssl,
  loading,
  error,
  hasDomains,
}: {
  name: string;
  ssl?: SSLStatus;
  loading: boolean;
  error: Error | null;
  hasDomains: boolean;
}) {
  const t = strings.domains;
  const canWrite = useCan("app.ssl.write");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [email, setEmail] = useState<string | null>(null);

  const change = useMutation({
    mutationFn: (req: SSLRequest) => changeAppSSL(name, req),
    onSuccess: async (res, req) => {
      if ("job_id" in res) {
        await navigate(`/jobs/${res.job_id}`);
        return;
      }
      qc.setQueryData(queryKeys.appSSL(name), res);
      if (req.action === "set_email") {
        setEmail(null);
        notify(t.emailSaved);
      }
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  if (loading) return <Skeleton className="h-48 rounded-xl" />;
  if (error) return <Banner variant="error">{errorMessage(error)}</Banner>;
  if (!ssl) return null;
  if (!ssl.available) {
    return (
      <Card>
        <CardHeader title={t.sslTitle} />
        <CardBody>
          <p className="font-semibold">{t.missingTitle}</p>
          <p className="mt-1 text-muted">
            {t.missingBody}{" "}
            <Link to="/plugins" className="font-semibold text-accent hover:underline">
              {t.seePlugins}
            </Link>
          </p>
        </CardBody>
      </Card>
    );
  }

  const shownEmail = email ?? ssl.email ?? "";
  return (
    <Card>
      <CardHeader title={t.sslTitle} />
      <div className="divide-y divide-line">
        <Row title={t.plugin} hint={t.pluginDetected}>
          <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-running-fg">
            <Check className="size-4" aria-hidden />
            {t.installed}
          </span>
        </Row>
        <Row
          title={strings.domains.ssl}
          hint={t.status(ssl.active, ssl.expires_at ? timeAgo(ssl.expires_at) : null)}
        >
          {ssl.expires_at ? (
            <time dateTime={ssl.expires_at} className="text-sm text-muted">
              {dateTime(ssl.expires_at)}
            </time>
          ) : null}
        </Row>
        {ssl.uncovered.length > 0 ? (
          <div className="px-5 py-3">
            <Banner variant="error">{t.uncovered(ssl.uncovered)}</Banner>
          </div>
        ) : null}
        <Row title={t.email} hint={t.emailHint}>
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              change.mutate({ action: "set_email", email: shownEmail });
            }}
          >
            <Field label={t.email} className="w-64">
              <Input
                type="email"
                value={shownEmail}
                readOnly={!canWrite}
                onChange={(e) => setEmail(e.target.value)}
              />
            </Field>
            {canWrite ? (
              <Button type="submit" variant="secondary" disabled={email === null || change.isPending}>
                {t.saveEmail}
              </Button>
            ) : null}
          </form>
        </Row>
        <Row title={t.autoRenew} hint={t.autoRenewHint}>
          <label className="flex min-h-11 items-center gap-2">
            <input
              type="checkbox"
              checked={ssl.auto_renew}
              disabled={!canWrite || change.isPending}
              onChange={(e) => change.mutate({ action: "set_auto_renew", auto_renew: e.target.checked })}
              className="size-[18px] accent-accent"
            />
            {t.renewAutomatically}
          </label>
        </Row>
      </div>
      {canWrite ? (
        <div className="flex flex-wrap gap-3 border-t border-line px-5 py-4">
          {ssl.active ? (
            <>
              <Button onClick={() => change.mutate({ action: "renew" })} disabled={change.isPending}>
                <RefreshCw aria-hidden />
                {t.renew}
              </Button>
              <Button
                variant="danger"
                onClick={() => change.mutate({ action: "disable" })}
                disabled={change.isPending}
              >
                <ShieldOff aria-hidden />
                {t.disable}
              </Button>
            </>
          ) : (
            <Button
              onClick={() => change.mutate({ action: "enable" })}
              disabled={change.isPending || !hasDomains}
            >
              <Lock aria-hidden />
              {t.enable}
            </Button>
          )}
        </div>
      ) : null}
    </Card>
  );
}

function Row({ title, hint, children }: { title: string; hint: string; children?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
      <div className="min-w-0 flex-[1_1_280px]">
        <div className="font-semibold">{title}</div>
        <div className="text-sm text-muted">{hint}</div>
      </div>
      <div className="flex-none">{children}</div>
    </div>
  );
}
