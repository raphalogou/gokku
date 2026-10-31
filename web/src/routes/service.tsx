import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { DatabaseZap, FolderKanban, Link2, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { type ReactNode, useState } from "react";
import { isRouteErrorResponse, Link, useNavigate, useRouteError } from "react-router";
import { Banner } from "~/components/app/banner";
import { Breadcrumbs } from "~/components/app/breadcrumbs";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { TypedConfirmDialog } from "~/components/app/typed-confirm-dialog";
import { ValueField } from "~/components/app/value-field";
import { Button, buttonVariants } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Field } from "~/components/ui/field";
import { Select } from "~/components/ui/select";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { ApiError } from "~/lib/api/fetcher";
import {
  destroyService,
  linkService,
  restartService,
  revealService,
  startService,
  stopService,
  unlinkService,
} from "~/lib/api/gen/client";
import type { JobRef, ServiceDetail } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { appsQuery, queryKeys, serverQuery, serviceQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { datastoreName, serviceStatusKind } from "~/lib/status";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/service";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  void queryClient.prefetchQuery(appsQuery());
  await queryClient.ensureQueryData(serviceQuery(params.type, params.name));
  return null;
}

type Action = "restart" | "stop" | "start";

const run: Record<Action, (type: string, name: string) => Promise<JobRef>> = {
  restart: restartService,
  stop: stopService,
  start: startService,
};

/** Service detail (SVC-2 to SVC-4): status and actions, masked connection with reveal,
 * linked apps with link and unlink, and destroy. */
export default function Service({ params }: Route.ComponentProps) {
  const { data: svc } = useSuspenseQuery(serviceQuery(params.type, params.name));
  const canOperate = useCan("service.operate");
  const canDestroy = useCan("service.destroy");
  const t = strings.service;
  const typeName = datastoreName(svc.type);

  return (
    <>
      <Breadcrumbs items={[{ to: "/services", label: t.breadcrumb }, { label: svc.name }]} />
      <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="font-mono text-[28px] leading-tight font-bold break-all">{svc.name}</h1>
            <StatusPill kind={serviceStatusKind[svc.status]} label={strings.serviceStatus[svc.status]} />
            {svc.project ? (
              <Link
                to={`/projects/${svc.project.slug}`}
                className="inline-flex min-h-7 items-center gap-1.5 rounded-full border border-input-border px-2.5 text-[13px] font-semibold hover:bg-subtle"
              >
                <FolderKanban className="size-3.5 text-muted" aria-hidden />
                {svc.project.name}
              </Link>
            ) : null}
          </div>
          <p className="mt-1 text-[15px] text-muted">{t.subtitle(typeName)}</p>
        </div>
        {canOperate ? <ServiceActions svc={svc} /> : null}
      </header>

      {/* One column on phones, with the linked apps before the danger zone; two on desktop. */}
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <div className="flex min-w-0 flex-col gap-6">
          {!canOperate ? <Banner>{t.readOnly}</Banner> : null}
          <Details svc={svc} />
          <Connection svc={svc} />
        </div>
        <div className="min-w-0 lg:col-start-2 lg:row-span-2 lg:row-start-1">
          <LinkedApps svc={svc} />
        </div>
        {canDestroy ? (
          <div className="min-w-0">
            <DangerZone svc={svc} />
          </div>
        ) : null}
      </div>
    </>
  );
}

function useJobMutation<V>(fn: (v: V) => Promise<JobRef>, message: (v: V) => string) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: fn,
    onSuccess: async ({ job_id }, v) => {
      notify(message(v));
      void qc.invalidateQueries({ queryKey: queryKeys.services() });
      await navigate(`/jobs/${job_id}`);
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });
}

function ServiceActions({ svc }: { svc: ServiceDetail }) {
  const { data: server } = useQuery(serverQuery());
  const [confirmStop, setConfirmStop] = useState(false);
  const t = strings.service;
  const start = useJobMutation(
    (action: Action) => run[action](svc.type, svc.name),
    (action) => t.actionStarted(t[action], svc.name),
  );
  const unreachable = server ? !server.reachable : false;
  const disabled = unreachable || start.isPending;
  const title = unreachable ? strings.actions.unreachable : undefined;
  const running = svc.status === "running";
  return (
    <div className="flex flex-wrap gap-3">
      <Button variant="secondary" onClick={() => start.mutate("restart")} disabled={disabled} title={title}>
        <RotateCw aria-hidden />
        {t.restart}
      </Button>
      {running ? (
        <Button variant="secondary" onClick={() => setConfirmStop(true)} disabled={disabled} title={title}>
          <Square aria-hidden />
          {t.stop}
        </Button>
      ) : (
        <Button onClick={() => start.mutate("start")} disabled={disabled} title={title}>
          <Play aria-hidden />
          {t.start}
        </Button>
      )}
      <Dialog
        open={confirmStop}
        onOpenChange={setConfirmStop}
        title={t.confirmStopTitle(svc.name)}
        description={t.confirmStopBody}
      >
        <DialogActions>
          <Button variant="secondary" onClick={() => setConfirmStop(false)}>
            {strings.common.cancel}
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              setConfirmStop(false);
              start.mutate("stop");
            }}
          >
            {t.stop}
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  );
}

function Item({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-[13px] font-semibold text-muted">{label}</dt>
      <dd className={mono ? "font-mono text-sm break-all" : "break-words"}>{children}</dd>
    </div>
  );
}

function Details({ svc }: { svc: ServiceDetail }) {
  const t = strings.service;
  return (
    <Card>
      <CardHeader title={t.details} />
      <CardBody>
        <dl className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <Item label={t.type}>{datastoreName(svc.type)}</Item>
          <Item label={t.status}>{strings.serviceStatus[svc.status]}</Item>
          <Item label={t.project}>{svc.project?.name ?? strings.apps.unassigned}</Item>
          <Item label={t.version} mono>
            {svc.version || "—"}
          </Item>
          <Item label={t.linkEnv} mono>
            {svc.link_env}
          </Item>
        </dl>
      </CardBody>
    </Card>
  );
}

function Connection({ svc }: { svc: ServiceDetail }) {
  const canReveal = useCan("service.secrets.reveal");
  const [revealed, setRevealed] = useState<string | null>(null);
  const t = strings.service;
  const c = svc.connection;
  const read = async () => {
    try {
      return (await revealService(svc.type, svc.name)).dsn;
    } catch (err) {
      notify(errorMessage(err), "error");
      return undefined;
    }
  };
  return (
    <Card>
      <CardHeader title={t.connection} />
      <CardBody>
        {c ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted" id="connection-hint">
              {t.connectionHint}
            </p>
            <ValueField
              name={t.connectionUrl}
              value={revealed ?? c.masked}
              hidden={revealed === null}
              secret
              readOnly
              describedBy="connection-hint"
              onChange={() => {}}
              onReveal={
                canReveal
                  ? () => {
                      void read().then((v) => v !== undefined && setRevealed(v));
                    }
                  : undefined
              }
              onHide={() => setRevealed(null)}
              readHidden={canReveal ? read : undefined}
            />
            <dl className="grid grid-cols-1 gap-5 sm:grid-cols-2">
              <Item label={t.host} mono>
                {c.host}
              </Item>
              <Item label={t.port} mono>
                {c.port ?? "—"}
              </Item>
              <Item label={t.username} mono>
                {c.username ?? "—"}
              </Item>
              <Item label={t.database} mono>
                {c.database ?? "—"}
              </Item>
            </dl>
          </div>
        ) : (
          <p className="text-muted">{t.connectionUnavailable}</p>
        )}
      </CardBody>
    </Card>
  );
}

function LinkedApps({ svc }: { svc: ServiceDetail }) {
  const canLink = useCan("service.link");
  const apps = useQuery(appsQuery());
  const t = strings.service;
  const [app, setApp] = useState("");
  const [error, setError] = useState<string | null>(null);
  const link = useJobMutation(
    (name: string) => linkService(svc.type, svc.name, { app: name }),
    (name) => t.linkStarted(name),
  );
  const unlink = useJobMutation(
    (name: string) => unlinkService(svc.type, svc.name, { app: name }),
    (name) => t.unlinkStarted(name),
  );
  // Only apps of the service's project, or unassigned apps for an unassigned service (SVC-3).
  const linkable = (apps.data ?? []).filter(
    (a) => (a.project?.slug ?? null) === (svc.project?.slug ?? null) && !svc.links.includes(a.name),
  );
  const chosen = app && linkable.some((a) => a.name === app) ? app : (linkable[0]?.name ?? "");

  return (
    <Card>
      <CardHeader title={t.linkedApps} />
      {svc.links.length === 0 ? (
        <p className="px-5 py-6 text-muted">{t.noLinks}</p>
      ) : (
        <DataTable label={t.linkedApps}>
          <THead>
            <TH>{t.app}</TH>
            {canLink ? (
              <TH>
                <span className="sr-only">{t.unlink}</span>
              </TH>
            ) : null}
          </THead>
          <tbody>
            {svc.links.map((name) => (
              <TR key={name}>
                <TD>
                  <Link to={`/apps/${name}`} className="font-semibold hover:underline">
                    {name}
                  </Link>
                </TD>
                {canLink ? (
                  <TD className="w-0 text-right">
                    <Button
                      variant="secondary"
                      aria-label={t.unlinkLabel(name)}
                      onClick={() => unlink.mutate(name)}
                      disabled={unlink.isPending}
                    >
                      {t.unlink}
                    </Button>
                  </TD>
                ) : null}
              </TR>
            ))}
          </tbody>
        </DataTable>
      )}
      {canLink ? (
        <form
          className="flex flex-wrap items-end gap-3 border-t border-line bg-subtle/40 px-5 py-4"
          onSubmit={(e) => {
            e.preventDefault();
            setError(null);
            if (chosen) link.mutate(chosen, { onError: (err) => setError(errorMessage(err)) });
          }}
        >
          {linkable.length === 0 ? (
            <p className="text-sm text-muted">{svc.project ? t.noLinkable : t.noLinkableUnassigned}</p>
          ) : (
            <>
              <Field label={t.linkAnother} error={error ?? undefined} className="flex-[1_1_200px]">
                <Select
                  value={chosen}
                  onValueChange={setApp}
                  options={linkable.map((a) => ({ value: a.name, label: a.name }))}
                />
              </Field>
              <Button type="submit" variant="secondary" disabled={link.isPending || !chosen}>
                <Link2 aria-hidden />
                {t.linkApp}
              </Button>
              <p className="basis-full text-[13px] text-muted">{t.linkHint}</p>
            </>
          )}
        </form>
      ) : null}
    </Card>
  );
}

function DangerZone({ svc }: { svc: ServiceDetail }) {
  const [open, setOpen] = useState(false);
  const qc = useQueryClient();
  const navigate = useNavigate();
  const t = strings.service;
  const destroy = useMutation({
    mutationFn: () => destroyService(svc.type, svc.name, { confirm: svc.name }),
    onSuccess: async ({ job_id }) => {
      setOpen(false);
      void qc.invalidateQueries({ queryKey: queryKeys.services() });
      await navigate(`/jobs/${job_id}`);
    },
  });
  const linked = svc.links.length > 0;
  return (
    <section className="rounded-xl border border-failed bg-surface" aria-labelledby="danger-zone">
      <h2
        id="danger-zone"
        className="flex min-h-12 items-center border-b border-failed px-5 py-3 text-base font-bold text-failed-fg"
      >
        {t.danger}
      </h2>
      <div className="flex flex-col items-start gap-4 p-5">
        <p className="max-w-2xl">{t.destroyBody}</p>
        {linked ? <Banner>{t.destroyLinked(svc.links.join(", "))}</Banner> : null}
        <Button variant="danger" onClick={() => setOpen(true)} disabled={linked}>
          <Trash2 aria-hidden />
          {t.destroy}
        </Button>
      </div>
      <TypedConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t.destroyTitle(svc.name)}
        description={t.destroyDescription}
        confirmValue={svc.name}
        confirmLabel={t.destroy}
        onConfirm={() => destroy.mutate()}
        pending={destroy.isPending}
        error={destroy.error ? errorMessage(destroy.error) : undefined}
      />
    </section>
  );
}

export function ErrorBoundary() {
  const error = useRouteError();
  const notFound =
    (error instanceof ApiError && error.status === 404) ||
    (isRouteErrorResponse(error) && error.status === 404);
  return (
    <EmptyState
      icon={DatabaseZap}
      title={notFound ? strings.service.notFoundTitle : strings.errors.loadFailed}
      action={
        <Link to="/services" className={buttonVariants({ variant: "secondary" })}>
          {strings.services.title}
        </Link>
      }
    >
      {notFound
        ? strings.service.notFoundBody
        : error instanceof ApiError
          ? error.message
          : strings.errors.unexpected}
    </EmptyState>
  );
}
