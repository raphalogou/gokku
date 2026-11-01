import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { useJobDock } from "~/components/app/job-dock";
import { TypedConfirmDialog } from "~/components/app/typed-confirm-dialog";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { Select } from "~/components/ui/select";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { ApiError } from "~/lib/api/fetcher";
import { destroyApp, updateApp } from "~/lib/api/gen/client";
import { errorMessage } from "~/lib/forms";
import { appQuery, projectsQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-settings";

/** App settings: rename (APP-4), move (APP-7) and destroy (APP-5), for admins. */
export default function AppSettings({ params }: Route.ComponentProps) {
  const { data: app } = useSuspenseQuery(appQuery(params.name));
  const projects = useQuery(projectsQuery());
  const canRename = useCan("app.rename");
  const canMove = useCan("app.move");
  const canDestroy = useCan("app.destroy");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const dock = useJobDock();
  const t = strings.appSettings;

  const [name, setName] = useState(app.name);
  const [project, setProject] = useState(app.project?.slug ?? "");
  const [nameError, setNameError] = useState<string>();
  const [destroyOpen, setDestroyOpen] = useState(false);

  const rename = useMutation({
    mutationFn: () => updateApp(app.name, { name }),
    onSuccess: async (res) => {
      if (res && "job_id" in res) {
        dock.track(res.job_id);
        await navigate("/apps");
      }
    },
    onError: (err) =>
      setNameError(err instanceof ApiError ? (err.fields.name ?? err.message) : errorMessage(err)),
  });
  const move = useMutation({
    mutationFn: () => updateApp(app.name, { project }),
    onSuccess: async (res) => {
      await qc.invalidateQueries({ queryKey: queryKeys.app(app.name) });
      void qc.invalidateQueries({ queryKey: queryKeys.projects() });
      notify(t.moved(projects.data?.find((p) => p.slug === project)?.name ?? project));
      if (res && "job_id" in res) dock.track(res.job_id);
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });
  const destroy = useMutation({
    mutationFn: () => destroyApp(app.name, { confirm: app.name }),
    onSuccess: async ({ job_id }) => {
      setDestroyOpen(false);
      dock.track(job_id);
      await navigate("/apps");
    },
  });

  return (
    <div className="flex max-w-4xl flex-col gap-6">
      {!canRename ? <Banner>{t.readOnly}</Banner> : null}
      <Card>
        <CardHeader title={t.general} />
        <div className="divide-y divide-line">
          <Row title={t.nameTitle} hint={t.nameHint}>
            <form
              className="flex flex-wrap items-end gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                setNameError(undefined);
                rename.mutate();
              }}
            >
              <Field label={t.nameTitle} error={nameError} className="w-56">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value.trim())}
                  readOnly={!canRename}
                  autoCapitalize="none"
                  spellCheck={false}
                  className="font-mono"
                />
              </Field>
              {canRename ? (
                <Button
                  type="submit"
                  variant="secondary"
                  disabled={name === app.name || !name || rename.isPending}
                >
                  {rename.isPending ? t.renaming : t.rename}
                </Button>
              ) : null}
            </form>
          </Row>
          <Row title={t.projectTitle} hint={t.projectHint}>
            <div className="flex flex-wrap items-end gap-3">
              <Field label={t.projectTitle} className="w-56">
                <Select
                  value={project}
                  onValueChange={setProject}
                  options={(projects.data ?? []).map((p) => ({ value: p.slug, label: p.name }))}
                />
              </Field>
              {canMove ? (
                <Button
                  variant="secondary"
                  onClick={() => move.mutate()}
                  disabled={!project || project === app.project?.slug || move.isPending}
                >
                  {t.move}
                </Button>
              ) : null}
            </div>
          </Row>
        </div>
      </Card>

      {canDestroy ? (
        <section className="rounded-xl border border-failed bg-surface" aria-labelledby="danger-zone">
          <h2
            id="danger-zone"
            className="flex min-h-12 items-center border-b border-failed px-5 py-3 text-base font-bold text-failed-fg"
          >
            {t.danger}
          </h2>
          <div className="flex flex-col items-start gap-4 p-5">
            <p className="max-w-2xl">{t.destroyBody}</p>
            <Button variant="danger" onClick={() => setDestroyOpen(true)}>
              <Trash2 aria-hidden />
              {t.destroy}
            </Button>
          </div>
        </section>
      ) : null}
      <TypedConfirmDialog
        open={destroyOpen}
        onOpenChange={setDestroyOpen}
        title={t.destroyTitle(app.name)}
        description={t.destroyDescription}
        confirmValue={app.name}
        confirmLabel={t.destroy}
        onConfirm={() => destroy.mutate()}
        pending={destroy.isPending}
        error={destroy.error ? errorMessage(destroy.error) : undefined}
      />
    </div>
  );
}

function Row({ title, hint, children }: { title: string; hint: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
      <div className="min-w-0 flex-[1_1_260px]">
        <div className="font-semibold">{title}</div>
        <div className="text-sm text-muted">{hint}</div>
      </div>
      <div className="flex-none">{children}</div>
    </div>
  );
}
