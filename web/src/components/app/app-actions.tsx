import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hammer, Play, RotateCw, Square } from "lucide-react";
import { useState } from "react";
import { useJobDock } from "~/components/app/job-dock";
import { useCan } from "~/hooks/use-can";
import { rebuildApp, restartApp, startApp, stopApp } from "~/lib/api/gen/client";
import type { AppDetail } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { queryKeys, serverQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";
import { Dialog, DialogActions } from "../ui/dialog";
import { notify } from "../ui/toast";

type Action = "restart" | "stop" | "start" | "rebuild";

const run: Record<Action, (name: string) => Promise<{ job_id: string }>> = {
  restart: restartApp,
  stop: stopApp,
  start: startApp,
  rebuild: rebuildApp,
};

/** Restart, rebuild, stop and start for operators (APP-6). Each opens its job's output. */
export function AppActions({ app }: { app: AppDetail }) {
  const canOperate = useCan("app.operate");
  const { data: server } = useQuery(serverQuery());
  const qc = useQueryClient();
  const dock = useJobDock();
  const [confirm, setConfirm] = useState<"stop" | "rebuild" | null>(null);
  const t = strings.actions;

  const start = useMutation({
    mutationFn: (action: Action) => run[action](app.name),
    onSuccess: async ({ job_id }, _action) => {
      setConfirm(null);
      void qc.invalidateQueries({ queryKey: queryKeys.app(app.name) });
      dock.track(job_id);
    },
    onError: (err) => {
      setConfirm(null);
      notify(errorMessage(err), "error");
    },
  });

  if (!canOperate) return null;
  const unreachable = server ? !server.reachable : false;
  const disabled = unreachable || start.isPending;
  const title = unreachable ? t.unreachable : undefined;
  const stopped = app.status === "stopped";

  return (
    <div className="flex flex-wrap gap-3">
      <Button
        variant="secondary"
        onClick={() => start.mutate("restart")}
        disabled={disabled || stopped}
        title={title}
      >
        <RotateCw aria-hidden />
        {t.restart}
      </Button>
      <Button variant="secondary" onClick={() => setConfirm("rebuild")} disabled={disabled} title={title}>
        <Hammer aria-hidden />
        {t.rebuild}
      </Button>
      {stopped ? (
        <Button onClick={() => start.mutate("start")} disabled={disabled} title={title}>
          <Play aria-hidden />
          {t.start}
        </Button>
      ) : (
        <Button variant="danger" onClick={() => setConfirm("stop")} disabled={disabled} title={title}>
          <Square aria-hidden />
          {t.stop}
        </Button>
      )}
      <Dialog
        open={confirm !== null}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={confirm === "stop" ? t.confirmStopTitle(app.name) : t.confirmRebuildTitle(app.name)}
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
    </div>
  );
}
