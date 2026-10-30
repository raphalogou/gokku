import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useOutletContext } from "react-router";
import {
  addProjectApp,
  addProjectService,
  removeProjectApp,
  removeProjectService,
} from "~/lib/api/gen/client";
import type { ProjectDetail, ProjectRef } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { appsQuery, queryKeys, servicesQuery } from "~/lib/queries";
import { datastoreName } from "~/lib/status";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";
import { Dialog, DialogActions } from "../ui/dialog";
import { Field } from "../ui/field";
import { Select } from "../ui/select";
import { notify } from "../ui/toast";
import { Banner } from "./banner";

export type MemberKind = "app" | "service";

/** What the project layout passes to its tabs. openAdd is set only for admins. */
export type ProjectOutletContext = { openAdd?: (kind: MemberKind) => void };

export function useProjectContext() {
  return useOutletContext<ProjectOutletContext>();
}

function where(project: ProjectRef | null | undefined) {
  return project ? strings.project.inProject(project.name) : strings.project.unassigned;
}

/** Refetch everything that shows membership. */
function useInvalidateMembership(slug: string) {
  const qc = useQueryClient();
  return () =>
    Promise.all(
      [queryKeys.project(slug), queryKeys.projects(), queryKeys.apps(), queryKeys.services()].map(
        (queryKey) => qc.invalidateQueries({ queryKey }),
      ),
    );
}

/** Adds an app or a service to the project, moving it from another one if needed (PRJ-2). */
export function AddMemberDialog({
  project,
  kind,
  onClose,
}: {
  project: ProjectDetail;
  kind: MemberKind;
  onClose: () => void;
}) {
  const apps = useQuery({ ...appsQuery(), enabled: kind === "app" });
  const services = useQuery({ ...servicesQuery(), enabled: kind === "service" });
  const invalidate = useInvalidateMembership(project.slug);

  const options =
    kind === "app"
      ? (apps.data ?? [])
          .filter((a) => a.project?.slug !== project.slug)
          .map((a) => ({ value: a.name, label: a.name, hint: where(a.project) }))
      : (services.data ?? [])
          .filter((s) => s.project?.slug !== project.slug)
          .map((s) => ({
            value: `${s.type}/${s.name}`,
            label: s.name,
            hint: `${datastoreName(s.type)}, ${where(s.project)}`,
          }));
  const [picked, setPicked] = useState("");
  const value = picked || options[0]?.value || "";
  const loading = kind === "app" ? apps.isPending : services.isPending;

  const add = useMutation({
    mutationFn: () => {
      if (kind === "app") return addProjectApp(project.slug, value);
      const [type = "", name = ""] = value.split("/");
      return addProjectService(project.slug, type, name);
    },
    onSuccess: async () => {
      await invalidate();
      notify(strings.project.added(value.split("/").at(-1) ?? value));
      onClose();
    },
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={kind === "app" ? strings.project.addAppTitle : strings.project.addServiceTitle}
      description={kind === "app" ? strings.project.addAppIntro : strings.project.addServiceIntro}
    >
      <div className="flex flex-col gap-5">
        {add.isError ? <Banner variant="error">{errorMessage(add.error)}</Banner> : null}
        {loading ? (
          <p className="text-muted">{strings.common.loading}</p>
        ) : options.length === 0 ? (
          <p className="text-muted">
            {kind === "app" ? strings.project.nothingToAdd : strings.project.nothingToAddService}
          </p>
        ) : (
          <Field label={kind === "app" ? strings.project.pickApp : strings.project.pickService}>
            <Select value={value} onValueChange={setPicked} options={options} />
          </Field>
        )}
      </div>
      <DialogActions>
        <Button variant="secondary" onClick={onClose}>
          {strings.common.cancel}
        </Button>
        <Button onClick={() => add.mutate()} disabled={!value || add.isPending}>
          {strings.project.add}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Releases an app or a service from the project; it keeps running, unassigned (PRJ-2). */
export function useRemoveMember(slug: string) {
  const invalidate = useInvalidateMembership(slug);
  return useMutation({
    mutationFn: ({ kind, type, name }: { kind: MemberKind; type?: string; name: string }) =>
      kind === "app" ? removeProjectApp(slug, name) : removeProjectService(slug, type ?? "", name),
    onSuccess: async (_, { name }) => {
      await invalidate();
      notify(strings.project.removed(name));
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });
}
