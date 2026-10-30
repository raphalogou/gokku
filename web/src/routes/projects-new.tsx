import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { data, useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Button } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Field } from "~/components/ui/field";
import { Input, Textarea } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { createProject } from "~/lib/api/gen/client";
import { applyServerError } from "~/lib/forms";
import { meQuery, queryKeys } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { projectSchema, slugify } from "~/lib/schemas";
import { strings } from "~/lib/strings";

export async function clientLoader() {
  const me = await queryClient.ensureQueryData(meQuery());
  if (me.role !== "admin") throw data(null, { status: 403 });
  return null;
}

/** The New project dialog, a modal route over the projects list (PRJ-1). */
export default function NewProject() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const form = useForm({
    resolver: zodResolver(projectSchema),
    defaultValues: { name: "", slug: "", description: "" },
  });
  const { errors, isSubmitting, dirtyFields } = form.formState;
  const close = () => void navigate("/projects");

  // The URL name follows the name until it is edited by hand.
  const nameField = form.register("name", {
    onChange: (e: { target: { value: string } }) => {
      if (!dirtyFields.slug) form.setValue("slug", slugify(e.target.value));
    },
  });

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const p = await createProject({
        name: values.name,
        slug: values.slug,
        description: values.description || undefined,
      });
      qc.setQueryData(queryKeys.project(p.slug), p);
      await qc.invalidateQueries({ queryKey: queryKeys.projects() });
      notify(strings.newProject.created(p.name));
      await navigate(`/projects/${p.slug}`);
    } catch (err) {
      applyServerError(err, form.setError, ["name", "slug", "description"]);
    }
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && close()}
      title={strings.newProject.title}
      description={strings.newProject.intro}
    >
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
        <Field label={strings.newProject.name} error={errors.name?.message}>
          <Input autoComplete="off" {...nameField} />
        </Field>
        <Field
          label={strings.newProject.slug}
          hint={strings.newProject.slugHint}
          error={errors.slug?.message}
        >
          <Input
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            className="font-mono"
            {...form.register("slug")}
          />
        </Field>
        <Field label={strings.newProject.description} error={errors.description?.message}>
          <Textarea rows={3} {...form.register("description")} />
        </Field>
        <DialogActions>
          <Button variant="secondary" onClick={close}>
            {strings.common.cancel}
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? strings.newProject.submitting : strings.newProject.submit}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
