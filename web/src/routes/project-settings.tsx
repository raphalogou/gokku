import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { TypedConfirmDialog } from "~/components/app/typed-confirm-dialog";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input, Textarea } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { ApiError } from "~/lib/api/fetcher";
import { deleteProject, updateProject } from "~/lib/api/gen/client";
import { errorMessage } from "~/lib/forms";
import { projectQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project-settings";

/** Project settings: name and description (PRJ-1), and deletion (PRJ-4). */
export default function ProjectSettings({ params }: Route.ComponentProps) {
  const { data: p } = useSuspenseQuery(projectQuery(params.slug));
  const canUpdate = useCan("project.update");
  const canDelete = useCan("project.delete");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const t = strings.projectSettings;
  const [name, setName] = useState(p.name);
  const [description, setDescription] = useState(p.description);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [deleteOpen, setDeleteOpen] = useState(false);

  const save = useMutation({
    mutationFn: () => updateProject(p.slug, { name: name.trim(), description: description.trim() }),
    onSuccess: (res) => {
      qc.setQueryData(queryKeys.project(p.slug), res);
      void qc.invalidateQueries({ queryKey: queryKeys.projects() });
      setFields({});
      notify(t.saved);
    },
    onError: (err) => setFields(err instanceof ApiError ? err.fields : { root: errorMessage(err) }),
  });
  const remove = useMutation({
    mutationFn: () => deleteProject(p.slug, { confirm: p.slug }),
    onSuccess: async (res) => {
      setDeleteOpen(false);
      qc.removeQueries({ queryKey: queryKeys.project(p.slug) });
      void qc.invalidateQueries({ queryKey: queryKeys.projects() });
      notify(t.deleted(p.name));
      await navigate(res && "job_id" in res ? `/jobs/${res.job_id}` : "/projects");
    },
  });
  const dirty = name !== p.name || description !== p.description;

  return (
    <div className="flex max-w-3xl flex-col gap-6">
      {!canUpdate ? <Banner>{t.readOnly}</Banner> : null}
      <Card>
        <CardHeader title={t.general} />
        <CardBody>
          <form
            noValidate
            className="flex flex-col gap-5"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            {fields.root ? <Banner variant="error">{fields.root}</Banner> : null}
            <Field label={t.name} error={fields.name}>
              <Input value={name} onChange={(e) => setName(e.target.value)} readOnly={!canUpdate} />
            </Field>
            <Field label={t.slug} hint={t.slugHint}>
              <Input value={p.slug} readOnly className="font-mono" />
            </Field>
            <Field label={t.description} error={fields.description}>
              <Textarea
                rows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                readOnly={!canUpdate}
              />
            </Field>
            {canUpdate ? (
              <div>
                <Button type="submit" disabled={!dirty || save.isPending}>
                  {save.isPending ? t.saving : t.save}
                </Button>
              </div>
            ) : null}
          </form>
        </CardBody>
      </Card>

      {canDelete ? (
        <section className="rounded-xl border border-failed bg-surface" aria-labelledby="danger-zone">
          <h2
            id="danger-zone"
            className="flex min-h-12 items-center border-b border-failed px-5 py-3 text-base font-bold text-failed-fg"
          >
            {t.danger}
          </h2>
          <div className="flex flex-col items-start gap-4 p-5">
            <p className="max-w-2xl">{t.deleteBody}</p>
            <Button variant="danger" onClick={() => setDeleteOpen(true)}>
              <Trash2 aria-hidden />
              {t.delete}
            </Button>
          </div>
        </section>
      ) : null}
      <TypedConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t.deleteTitle(p.name)}
        description={t.deleteDescription}
        confirmValue={p.slug}
        confirmLabel={t.delete}
        onConfirm={() => remove.mutate()}
        pending={remove.isPending}
        error={remove.error ? errorMessage(remove.error) : undefined}
      />
    </div>
  );
}
