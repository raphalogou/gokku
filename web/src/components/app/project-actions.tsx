import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hammer, Play, RotateCw, Square } from "lucide-react";
import { useState } from "react";
import { useJobDock } from "~/components/app/job-dock";
import { useCan } from "~/hooks/use-can";
import {
  rebuildProjectApps,
  restartProjectApps,
  startProjectApps,
  stopProjectApps,
} from "~/lib/api/gen/client";
import type { ProjectDetail } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { queryKeys, serverQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";
import { Dialog, DialogActions } from "../ui/dialog";
import { MenuItem } from "../ui/menu";
import { notify } from "../ui/toast";
import { RowMenu } from "./row-menu";

type Action = "restart" | "stop" | "start" | "rebuild";

const run: Record<Action, (slug: string) => Promise<{ job_id: string }>> = {
  restart: restartProjectApps,
  stop: stopProjectApps,
  start: startProjectApps,
  rebuild: rebuildProjectApps,
};

/** Restart, stop, start or rebuild every app of a project as one job (PRJ-5, JOB-5). */
export function ProjectActions({ project }: { project: ProjectDetail }) {
  const canOperate = useCan("project.operate");
  const { data: server } = useQuery(serverQuery());
  const qc = useQueryClient();
  const dock = useJobDock();
  const [confirm, setConfirm] = useState<"stop" | "rebuild" | null>(null);
  const t = strings.projectActions;

  const start = useMutation({
    mutationFn: (action: Action) => run[action](project.slug),
    onSuccess: async ({ job_id }, _action) => {
      setConfirm(null);
      void qc.invalidateQueries({ queryKey: queryKeys.apps() });
      dock.track(job_id);
    },
    onError: (err) => {
      setConfirm(null);
      notify(errorMessage(err), "error");
    },
  });

  if (!canOperate || project.apps.length === 0) return null;
  const unreachable = server ? !server.reachable : false;
  const disabled = unreachable || start.isPending;

  return (
    <>
      <Button
        variant="secondary"
        onClick={() => start.mutate("restart")}
        disabled={disabled}
        title={unreachable ? strings.actions.unreachable : undefined}
      >
        <RotateCw aria-hidden />
        {t.restart}
      </Button>
      <RowMenu name={project.name}>
        <MenuItem onClick={() => start.mutate("start")} disabled={disabled}>
          <Play aria-hidden />
          {t.start}
        </MenuItem>
        <MenuItem onClick={() => setConfirm("rebuild")} disabled={disabled}>
          <Hammer aria-hidden />
          {t.rebuild}
        </MenuItem>
        <MenuItem onClick={() => setConfirm("stop")} disabled={disabled}>
          <Square aria-hidden />
          {t.stop}
        </MenuItem>
      </RowMenu>
      <Dialog
        open={confirm !== null}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={confirm === "stop" ? t.confirmStopTitle(project.name) : t.confirmRebuildTitle(project.name)}
        description={confirm === "stop" ? t.confirmStopBody : t.confirmRebuildBody}
      >
        <DialogActions>
          <Button variant="secondary" onClick={() => setConfirm(null)}>
            {strings.common.cancel}
          </Button>
          <Button
            variant={confirm === "stop" ? "danger" : "primary"}
            onClick={() => confirm && start.mutate(confirm)}
            disabled={start.isPending}
          >
            {confirm === "stop" ? t.stop : t.rebuild}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
