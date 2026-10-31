import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { data, useNavigate, useSearchParams } from "react-router";
import { Banner } from "~/components/app/banner";
import { Button } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { Select } from "~/components/ui/select";
import { notify } from "~/components/ui/toast";
import { ApiError } from "~/lib/api/fetcher";
import { createService } from "~/lib/api/gen/client";
import { errorMessage } from "~/lib/forms";
import { meQuery, projectsQuery, queryKeys, serverQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { datastoreName } from "~/lib/status";
import { strings } from "~/lib/strings";

export async function clientLoader() {
  const me = await queryClient.ensureQueryData(meQuery());
  if (me.role !== "admin") throw data(null, { status: 403 });
  void queryClient.prefetchQuery(projectsQuery());
  return null;
}

const nameRe = /^[A-Za-z][A-Za-z0-9_-]*$/;

/** The Create service dialog, a modal route over the services list (SVC-2). */
export default function NewService() {
  const navigate = useNavigate();
  const [search] = useSearchParams();
  const qc = useQueryClient();
  const projects = useQuery(projectsQuery());
  const { data: server } = useQuery(serverQuery());
  const types = server?.datastores ?? [];
  const [type, setType] = useState("");
  const [name, setName] = useState("");
  const [project, setProject] = useState(search.get("project") ?? "");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const t = strings.newService;
  const close = () => void navigate("/services");

  const projectOptions = (projects.data ?? []).map((p) => ({ value: p.slug, label: p.name }));
  const chosenProject = project || projectOptions[0]?.value || "";
  const chosenType = type || types[0] || "";

  const create = useMutation({
    mutationFn: () => createService({ type: chosenType, name, project: chosenProject }),
    onSuccess: async ({ job_id }) => {
      notify(t.started(name));
      void qc.invalidateQueries({ queryKey: queryKeys.services() });
      await navigate(`/jobs/${job_id}`);
    },
    onError: (err) =>
      setErrors(
        err instanceof ApiError && Object.keys(err.fields).length ? err.fields : { root: errorMessage(err) },
      ),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && close()} title={t.title}>
      <form
        noValidate
        className="flex flex-col gap-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (!nameRe.test(name)) return setErrors({ name: t.nameHint });
          setErrors({});
          create.mutate();
        }}
      >
        {errors.root ? <Banner variant="error">{errors.root}</Banner> : null}
        {projects.isSuccess && projectOptions.length === 0 ? <Banner>{t.noProjects}</Banner> : null}
        <Field label={t.type} error={errors.type}>
          <Select
            value={chosenType}
            onValueChange={setType}
            options={types.map((ty) => ({ value: ty, label: datastoreName(ty) }))}
          />
        </Field>
        <Field label={t.name} hint={t.nameHint} error={errors.name}>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value.trim())}
            placeholder="app-db"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            className="font-mono"
          />
        </Field>
        <Field label={t.project} hint={t.projectHint} error={errors.project}>
          <Select value={chosenProject} onValueChange={setProject} options={projectOptions} />
        </Field>
        <DialogActions>
          <Button variant="secondary" onClick={close}>
            {strings.common.cancel}
          </Button>
          <Button type="submit" disabled={create.isPending || !chosenProject || !chosenType || !name}>
            {create.isPending ? t.submitting : t.submit}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
