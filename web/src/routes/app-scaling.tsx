import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { SaveBar } from "~/components/app/save-bar";
import { Skeleton } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button } from "~/components/ui/button";
import { Stepper } from "~/components/ui/stepper";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { putAppScale } from "~/lib/api/gen/client";
import { errorMessage } from "~/lib/forms";
import { appScaleQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-scaling";

/** Scaling (SCL-1): steppers per process type, then "Apply and restart" as a job. */
export default function AppScaling({ params }: Route.ComponentProps) {
  const name = params.name;
  const scale = useQuery(appScaleQuery(name));
  const canScale = useCan("app.scale");
  const navigate = useNavigate();
  const [draft, setDraft] = useState<Record<string, number>>({});
  const [error, setError] = useState<string | null>(null);
  const t = strings.scaling;

  const processes = scale.data?.processes ?? [];
  const changed = processes.filter((p) => p.type in draft && draft[p.type] !== p.quantity);
  const editable = canScale && !!scale.data?.can_scale;

  const apply = useMutation({
    mutationFn: () =>
      putAppScale(name, {
        processes: Object.fromEntries(changed.map((p) => [p.type, draft[p.type] ?? p.quantity])),
      }),
    onSuccess: async ({ job_id }) => {
      setDraft({});
      notify(t.started);
      await navigate(`/jobs/${job_id}`);
    },
    onError: (err) => setError(errorMessage(err)),
  });

  return (
    <div className="flex max-w-4xl flex-col gap-4">
      <p className="max-w-2xl text-muted">{t.intro}</p>
      {scale.data && !scale.data.can_scale ? <Banner>{t.locked}</Banner> : null}
      {!canScale ? <Banner>{t.readOnly}</Banner> : null}
      {error ? <Banner variant="error">{error}</Banner> : null}
      <Card>
        <CardHeader title={t.title} />
        {scale.isPending ? (
          <div className="p-5">
            <Skeleton className="h-28" />
          </div>
        ) : scale.isError ? (
          <div className="p-5">
            <Banner variant="error">{errorMessage(scale.error)}</Banner>
          </div>
        ) : processes.length === 0 ? (
          <p className="px-5 py-6 text-muted">{t.empty}</p>
        ) : (
          <DataTable label={t.title} className="min-w-[640px]">
            <THead>
              <TH>{t.type}</TH>
              <TH>{t.instances}</TH>
              <TH>{t.status}</TH>
              <TH>{t.pending}</TH>
            </THead>
            <tbody>
              {processes.map((p) => {
                const value = draft[p.type] ?? p.quantity;
                return (
                  <TR key={p.type}>
                    <TD className="font-mono font-medium">{p.type}</TD>
                    <TD>
                      <Stepper
                        label={t.instancesOf(p.type)}
                        value={value}
                        onChange={(n) => setDraft((prev) => ({ ...prev, [p.type]: n }))}
                        disabled={!editable}
                      />
                    </TD>
                    <TD>
                      <StatusPill
                        kind={p.running === 0 ? "stopped" : p.running < p.quantity ? "deploying" : "running"}
                        label={strings.app.processRunning(p.running, p.quantity)}
                      />
                    </TD>
                    <TD>
                      {value === p.quantity ? (
                        <span className="text-[13px] text-muted">{t.noChange}</span>
                      ) : (
                        <span className="text-[13px] font-semibold text-deploying-fg">
                          {t.changedFrom(p.quantity)}
                        </span>
                      )}
                    </TD>
                  </TR>
                );
              })}
            </tbody>
          </DataTable>
        )}
      </Card>
      <SaveBar
        count={changed.length}
        onDiscard={() => {
          setDraft({});
          setError(null);
        }}
      >
        <Button onClick={() => apply.mutate()} disabled={apply.isPending}>
          {apply.isPending ? t.applying : t.apply}
        </Button>
      </SaveBar>
    </div>
  );
}
