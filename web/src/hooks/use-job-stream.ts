import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { getStreamJobUrl } from "~/lib/api/gen/client";
import type { JobDetail, JobLine, JobStep } from "~/lib/api/gen/model";
import { type ConsoleLine, clockTime, detectLevel } from "~/lib/console";
import { queryKeys } from "~/lib/queries";

function toConsole(l: JobLine, steps: JobStep[]): ConsoleLine {
  const { level, text } =
    l.stream === "system"
      ? { level: l.text.startsWith("Failed") ? ("ERROR" as const) : ("INFO" as const), text: l.text }
      : detectLevel(l.text, l.stream);
  const step = l.step != null ? steps.find((s) => s.position === l.step)?.name : undefined;
  return { key: String(l.seq), time: clockTime(l.ts), label: step ?? null, level, text };
}

/**
 * Follows a job's output over SSE (JOB-3). Lines go to push; step and status changes update
 * the cached job. The browser resumes after the last seq with Last-Event-ID if the
 * connection drops. The stream closes when the job ends.
 */
export function useJobStream(job: JobDetail | undefined, push: (lines: ConsoleLine[]) => void) {
  const qc = useQueryClient();
  const [connected, setConnected] = useState(false);
  const id = job?.id;
  const finished = job ? !["queued", "running"].includes(job.status) : true;
  // The stream opens once per job; the steps only name the lines' steps.
  const steps = useRef(job?.steps ?? []);
  steps.current = job?.steps ?? steps.current;

  useEffect(() => {
    if (!id) return;
    const source = new EventSource(getStreamJobUrl(id));
    let currentSteps = steps.current;
    source.onopen = () => setConnected(true);
    source.onerror = () => setConnected(false);
    source.addEventListener("line", (e) => {
      push([toConsole(JSON.parse((e as MessageEvent<string>).data) as JobLine, currentSteps)]);
    });
    source.addEventListener("step", (e) => {
      const step = JSON.parse((e as MessageEvent<string>).data) as JobStep;
      currentSteps = currentSteps.map((s) => (s.position === step.position ? step : s));
      qc.setQueryData<JobDetail>(queryKeys.job(id), (j) =>
        j ? { ...j, status: "running", steps: currentSteps } : j,
      );
    });
    source.addEventListener("status", (e) => {
      const final = JSON.parse((e as MessageEvent<string>).data) as JobDetail;
      qc.setQueryData(queryKeys.job(id), final);
      source.close();
      setConnected(false);
      void qc.invalidateQueries({ queryKey: queryKeys.jobs() });
      if (final.app) void qc.invalidateQueries({ queryKey: queryKeys.app(final.app) });
    });
    return () => source.close();
  }, [id, qc, push]);

  return { connected, finished };
}
