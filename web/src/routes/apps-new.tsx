import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { data, Link, useNavigate, useSearchParams } from "react-router";
import { Banner } from "~/components/app/banner";
import { CodeBlock } from "~/components/app/code-block";
import { Button, buttonVariants } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { Select } from "~/components/ui/select";
import { ApiError } from "~/lib/api/fetcher";
import { createApp } from "~/lib/api/gen/client";
import type { AppCreated, AppSource } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { meQuery, projectsQuery, queryKeys, serverQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";

export async function clientLoader() {
  const me = await queryClient.ensureQueryData(meQuery());
  if (me.role !== "admin") throw data(null, { status: 403 });
  void queryClient.prefetchQuery(projectsQuery());
  return null;
}

const nameRe = /^[a-z0-9][a-z0-9.-]*$/;
const sources: AppSource[] = ["git", "image", "repository"];

/** The New app dialog, a modal route over the apps list (APP-3). */
export default function NewApp() {
  const navigate = useNavigate();
  const [search] = useSearchParams();
  const qc = useQueryClient();
  const projects = useQuery(projectsQuery());
  const { data: server } = useQuery(serverQuery());
  const [name, setName] = useState("");
  const [project, setProject] = useState(search.get("project") ?? "");
  const [source, setSource] = useState<AppSource>("git");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [created, setCreated] = useState<AppCreated | null>(null);
  const t = strings.newApp;
  const close = () => void navigate(created ? `/apps/${created.app.name}` : "/apps");

  const options = (projects.data ?? []).map((p) => ({ value: p.slug, label: p.name }));
  const chosen = project || options[0]?.value || "";

  const create = useMutation({
    mutationFn: () => createApp({ name, project: chosen, source }),
    onSuccess: (res) => {
      setCreated(res);
      void qc.invalidateQueries({ queryKey: queryKeys.apps() });
      void qc.invalidateQueries({ queryKey: queryKeys.projects() });
    },
    onError: (err) =>
      setErrors(
        err instanceof ApiError && Object.keys(err.fields).length ? err.fields : { root: errorMessage(err) },
      ),
  });

  if (created) {
    return (
      <Dialog open onOpenChange={(open) => !open && close()} title={t.createdTitle(created.app.name)}>
        {created.source === "git" ? (
          <div className="flex flex-col gap-3">
            <p>{t.gitIntro}</p>
            <CodeBlock value={`git remote add dokku ${created.git_remote}`} label={t.remote} />
            <CodeBlock value="git push dokku main" label="git push" />
          </div>
        ) : (
          <p>{t.deployLater}</p>
        )}
        <DialogActions>
          <Link to={`/apps/${created.app.name}`} className={buttonVariants()}>
            {t.open}
          </Link>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && close()}
      title={t.title}
      description={t.intro(server?.name ?? "")}
    >
      <form
        noValidate
        className="flex flex-col gap-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (!nameRe.test(name)) return setErrors({ name: t.nameInvalid });
          setErrors({});
          create.mutate();
        }}
      >
        {errors.root ? <Banner variant="error">{errors.root}</Banner> : null}
        {projects.isSuccess && options.length === 0 ? <Banner>{t.noProjects}</Banner> : null}
        <Field label={t.name} hint={t.nameHint} error={errors.name}>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value.trim())}
            placeholder="my-app"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            className="font-mono"
          />
        </Field>
        <Field label={t.project} error={errors.project}>
          <Select value={chosen} onValueChange={setProject} options={options} />
        </Field>
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-2 text-sm font-semibold">{t.source}</legend>
          {sources.map((s) => (
            <label
              key={s}
              className={cn(
                "flex min-h-11 cursor-pointer items-start gap-3 rounded-[10px] border px-3.5 py-3",
                source === s ? "border-accent bg-nav-active" : "border-input-border bg-surface",
              )}
            >
              <input
                type="radio"
                name="source"
                value={s}
                checked={source === s}
                onChange={() => setSource(s)}
                className="mt-1 size-[18px] accent-accent"
              />
              <span className="flex flex-col">
                <span className="font-semibold">{t.sources[s].label}</span>
                <span className="text-sm text-muted">{t.sources[s].hint}</span>
              </span>
            </label>
          ))}
        </fieldset>
        <DialogActions>
          <Button variant="secondary" onClick={close}>
            {strings.common.cancel}
          </Button>
          <Button type="submit" disabled={create.isPending || !chosen || !name}>
            {create.isPending ? t.submitting : t.submit}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
