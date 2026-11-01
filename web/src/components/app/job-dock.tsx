import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronUp, CircleSlash, Maximize2, Minus, RefreshCw, TriangleAlert, X } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useLocation } from "react-router";
import { useConsoleBuffer } from "~/hooks/use-console-buffer";
import { useJobStream } from "~/hooks/use-job-stream";
import type { JobDetail, JobStatus } from "~/lib/api/gen/model";
import { duration } from "~/lib/format";
import { jobQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";

/**
 * The job dock: jobs started from any screen show their progress in a corner panel, so
 * the user stays where they were (JOB-3). It lives in the shell and survives navigation.
 * The tracked ids are kept for the browser tab only; the jobs themselves are on the server.
 */
type Dock = {
  /** Shows a job in the dock, expanded, on top. */
  track: (id: string) => void;
};

const DockContext = createContext<Dock | null>(null);

const storageKey = "gokku.jobDock";
const maxJobs = 5;

function loadIds(): string[] {
  try {
    const raw = sessionStorage.getItem(storageKey);
    const ids: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(ids) ? ids.filter((x): x is string => typeof x === "string").slice(0, maxJobs) : [];
  } catch {
    return [];
  }
}

function saveIds(ids: string[]) {
  try {
    sessionStorage.setItem(storageKey, JSON.stringify(ids));
  } catch {
    // Storage can be blocked; the dock then forgets its jobs on reload.
  }
}

export function JobDockProvider({ children }: { children: ReactNode }) {
  const [ids, setIds] = useState<string[]>(loadIds);
  const [minimized, setMinimized] = useState(false);
  const [finished, setFinished] = useState<Record<string, boolean>>({});

  useEffect(() => saveIds(ids), [ids]);

  const track = useCallback((id: string) => {
    setIds((prev) => [id, ...prev.filter((x) => x !== id)].slice(0, maxJobs));
    setMinimized(false);
  }, []);
  const dismiss = useCallback((id: string) => setIds((prev) => prev.filter((x) => x !== id)), []);
  const onStatus = useCallback(
    (id: string, done: boolean) =>
      setFinished((prev) => (prev[id] === done ? prev : { ...prev, [id]: done })),
    [],
  );
  const clearFinished = () => setIds((prev) => prev.filter((id) => !finished[id]));
  const value = useMemo(() => ({ track }), [track]);
  // A job page shows the job in full: the dock steps aside to a pill there.
  const { pathname } = useLocation();
  const onJobPage = /\/(jobs|deploys)\/[^/]+$/.test(pathname);
  const [forceOpen, setForceOpen] = useState(false);
  // biome-ignore lint/correctness/useExhaustiveDependencies: reset when the page changes
  useEffect(() => setForceOpen(false), [pathname]);
  const collapsed = minimized || (onJobPage && !forceOpen);
  const running = ids.filter((id) => !finished[id]).length;
  const t = strings.jobDock;

  return (
    <DockContext.Provider value={value}>
      {children}
      {ids.length > 0 ? (
        <section
          aria-label={t.label}
          className="fixed inset-x-3 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-40 nav:inset-x-auto nav:right-6 nav:bottom-6 nav:w-[400px]"
        >
          {collapsed ? (
            <button
              type="button"
              onClick={() => {
                setMinimized(false);
                if (onJobPage) setForceOpen(true);
              }}
              aria-label={t.expand}
              className="ml-auto flex min-h-11 cursor-pointer items-center gap-2 rounded-full border border-line bg-surface px-4 font-semibold shadow-lg"
            >
              {running > 0 ? (
                <RefreshCw
                  className="size-4 animate-spin text-deploying-fg [animation-duration:2s]"
                  aria-hidden
                />
              ) : (
                <Check className="size-4 text-running-fg" aria-hidden />
              )}
              {t.summary(ids.length, running)}
              <ChevronUp className="size-4 text-muted" aria-hidden />
            </button>
          ) : (
            <div className="overflow-hidden rounded-xl border border-line bg-surface shadow-lg">
              <div className="flex min-h-11 items-center gap-1 border-b border-line pr-1 pl-4">
                <h2 className="flex-1 text-[15px] font-bold">{t.title(ids.length, running)}</h2>
                <button
                  type="button"
                  onClick={() => setMinimized(true)}
                  aria-label={t.minimize}
                  className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle"
                >
                  <Minus className="size-[18px]" aria-hidden />
                </button>
                <button
                  type="button"
                  onClick={running > 0 ? clearFinished : () => setIds([])}
                  aria-label={running > 0 ? t.clearFinished : t.close}
                  title={running > 0 ? t.clearFinished : t.close}
                  className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle"
                >
                  <X className="size-[18px]" aria-hidden />
                </button>
              </div>
              <ul className="max-h-[60vh] overflow-y-auto">
                {ids.map((id, i) => (
                  <DockJob key={id} id={id} primary={i === 0} onStatus={onStatus} onDismiss={dismiss} />
                ))}
              </ul>
            </div>
          )}
        </section>
      ) : null}
    </DockContext.Provider>
  );
}

/** Starts following a job in the dock. */
export function useJobDock(): Dock {
  const dock = useContext(DockContext);
  if (!dock) throw new Error("useJobDock needs a JobDockProvider");
  return dock;
}

const isFinished = (s: JobStatus) => s !== "queued" && s !== "running";

/** Where the full output of a job lives: deploys have their own page under the app. */
export function jobHref(job: Pick<JobDetail, "id" | "kind" | "app">): string {
  return job.app && job.kind.startsWith("deploy.") ? `/apps/${job.app}/deploys/${job.id}` : `/jobs/${job.id}`;
}

const icons: Record<JobStatus, { icon: typeof Check; className: string }> = {
  queued: { icon: RefreshCw, className: "text-muted" },
  running: { icon: RefreshCw, className: "animate-spin text-deploying-fg [animation-duration:2s]" },
  succeeded: { icon: Check, className: "text-running-fg" },
  failed: { icon: TriangleAlert, className: "text-failed-fg" },
  canceled: { icon: CircleSlash, className: "text-muted" },
};

function DockJob({
  id,
  primary,
  onStatus,
  onDismiss,
}: {
  id: string;
  primary: boolean;
  onStatus: (id: string, finished: boolean) => void;
  onDismiss: (id: string) => void;
}) {
  const qc = useQueryClient();
  // The top job streams its output; the others are polled while they run.
  const { data: job, isError } = useQuery({
    ...jobQuery(id),
    refetchInterval: (q) => (!primary && q.state.data && !isFinished(q.state.data.status) ? 2000 : false),
  });
  const done = job ? isFinished(job.status) : false;
  const wasDone = useRef<boolean | null>(null);

  useEffect(() => {
    if (!job) return;
    onStatus(id, done);
    // When a job ends, what it changed is shown fresh everywhere.
    if (wasDone.current === false && done)
      void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== "jobs" });
    wasDone.current = done;
  }, [id, job, done, onStatus, qc]);

  useEffect(() => {
    if (isError) onDismiss(id);
  }, [isError, id, onDismiss]);

  const t = strings.jobDock;
  if (!job) {
    return (
      <li className="flex items-center gap-3 border-b border-line px-4 py-3 text-muted last:border-b-0">
        <RefreshCw className="size-[18px] shrink-0 animate-spin [animation-duration:2s]" aria-hidden />
        {t.starting}
      </li>
    );
  }
  const { icon: Icon, className } = icons[job.status];
  const step =
    job.steps.find((s) => s.status === "running") ?? job.steps.findLast((s) => s.status !== "queued");
  const time = job.started_at ? duration(job.started_at, job.finished_at) : null;

  return (
    <li className="border-b border-line px-4 py-3 last:border-b-0">
      <div className="flex items-start gap-3">
        <Icon className={cn("mt-0.5 size-[18px] shrink-0", className)} aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">{job.title}</p>
          <p className="text-[13px] text-muted" aria-live="polite">
            {strings.job.status[job.status]}
            {step && !done ? ` · ${step.name}` : ""}
            {time ? ` · ${time}` : ""}
          </p>
        </div>
        <Link
          to={jobHref(job)}
          aria-label={t.open(job.title)}
          title={t.openShort}
          className="-my-2 flex size-11 shrink-0 items-center justify-center rounded-lg text-muted hover:bg-subtle"
        >
          <Maximize2 className="size-4" aria-hidden />
        </Link>
        {done ? (
          <button
            type="button"
            onClick={() => onDismiss(id)}
            aria-label={t.dismiss(job.title)}
            className="-my-2 -mr-2 flex size-11 shrink-0 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle"
          >
            <X className="size-4" aria-hidden />
          </button>
        ) : null}
      </div>
      {job.error && job.status === "failed" ? (
        <p className="mt-2 text-[13px] break-words text-failed-fg">{job.error}</p>
      ) : null}
      {primary && !job.children.length ? <Tail job={job} /> : null}
    </li>
  );
}

/** The last lines of a job's output. */
function Tail({ job }: { job: JobDetail }) {
  const buffer = useConsoleBuffer();
  useJobStream(job, buffer.push);
  const last = buffer.lines.slice(-3);
  if (last.length === 0) return null;
  return (
    <pre
      // A glance only: the status line above is announced, and the full output is one link away.
      aria-hidden="true"
      className="mt-2 overflow-hidden rounded-md bg-code-bg px-3 py-2 font-mono text-[12px] leading-5"
    >
      {last.map((l) => (
        <span key={l.key} className="block truncate">
          {l.text}
        </span>
      ))}
    </pre>
  );
}
