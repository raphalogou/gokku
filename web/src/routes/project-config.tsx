import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Eye, Plus, RefreshCw, Trash2, Undo2 } from "lucide-react";
import { useId, useState } from "react";
import { useNavigate } from "react-router";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { SaveBar } from "~/components/app/save-bar";
import { Skeleton } from "~/components/app/states";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { applyProjectEnv, putProjectEnv, revealProjectEnv } from "~/lib/api/gen/client";
import type { SharedVar, SharedVarInput } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { projectEnvQuery, projectQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project-config";

const keyRe = /^[A-Z_][A-Z0-9_]{0,127}$/;
const secretRe = /(secret|token|password|passwd|key|dsn|url|credential)/i;

type Draft = { key: string; value: string | null; secret: boolean; isNew: boolean };

/** Shared variables (SHV-1 to SHV-7): status per app, save and apply, reapply. */
export default function ProjectConfig({ params }: Route.ComponentProps) {
  const slug = params.slug;
  const { data: project } = useSuspenseQuery(projectQuery(slug));
  const env = useQuery(projectEnvQuery(slug));
  const canWrite = useCan("project.env.write");
  const canReveal = useCan("app.secrets.reveal");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const t = strings.sharedConfig;

  const [edits, setEdits] = useState<Record<string, string>>({});
  const [secretFlags, setSecretFlags] = useState<Record<string, boolean>>({});
  const [added, setAdded] = useState<Draft[]>([]);
  const [removed, setRemoved] = useState<Set<string>>(new Set());
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [restart, setRestart] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const vars = env.data?.vars ?? [];
  const changedKeys = vars
    .filter(
      (v) =>
        (v.key in edits && edits[v.key] !== (v.value ?? "")) ||
        (v.key in secretFlags && secretFlags[v.key] !== v.secret),
    )
    .map((v) => v.key);
  const count = changedKeys.length + added.length + removed.size;
  const discard = () => {
    setEdits({});
    setSecretFlags({});
    setAdded([]);
    setRemoved(new Set());
    setError(null);
  };

  const save = useMutation({
    mutationFn: () => {
      const out: SharedVarInput[] = [];
      for (const v of vars) {
        if (removed.has(v.key)) continue;
        const input: SharedVarInput = { key: v.key, secret: secretFlags[v.key] ?? v.secret };
        if (v.key in edits) input.value = edits[v.key];
        else if (!v.secret && v.value != null) input.value = v.value;
        out.push(input);
      }
      for (const d of added) out.push({ key: d.key, value: d.value ?? "", secret: d.secret });
      return putProjectEnv(slug, { vars: out, apply: true, restart });
    },
    onSuccess: async (res) => {
      qc.setQueryData(queryKeys.projectEnv(slug), res.env);
      discard();
      setRevealed({});
      notify(t.started);
      if (res.job_id) await navigate(`/jobs/${res.job_id}`);
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const reapply = useMutation({
    mutationFn: () => applyProjectEnv(slug, { restart }),
    onSuccess: async ({ job_id }) => {
      notify(t.started);
      await navigate(`/jobs/${job_id}`);
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const reveal = useMutation({
    mutationFn: (key: string) => revealProjectEnv(slug, { keys: [key] }),
    onSuccess: (r) => setRevealed((prev) => ({ ...prev, ...r.values })),
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const pending = vars.some((v) => v.apps.some((a) => a.status === "pending"));

  return (
    <div className="flex max-w-5xl flex-col gap-4">
      <p className="max-w-3xl text-muted">{t.intro(project.name)}</p>
      {!canWrite ? <Banner>{t.readOnly}</Banner> : null}
      {env.data && !env.data.reachable ? <Banner variant="error">{t.unknown}</Banner> : null}
      {error ? <Banner variant="error">{error}</Banner> : null}
      <Card>
        <CardHeader
          title={t.title}
          actions={
            canWrite && pending && count === 0 ? (
              <Button variant="secondary" onClick={() => reapply.mutate()} disabled={reapply.isPending}>
                <RefreshCw aria-hidden />
                {reapply.isPending ? t.reapplying : t.reapply}
              </Button>
            ) : null
          }
        />
        {env.isPending ? (
          <div className="flex flex-col gap-3 p-5" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-11" />
            ))}
          </div>
        ) : env.isError ? (
          <div className="p-5">
            <Banner variant="error">{errorMessage(env.error)}</Banner>
          </div>
        ) : (
          <DataTable label={t.title} className="min-w-[720px]">
            <THead>
              <TH className="w-[28%]">{t.key}</TH>
              <TH>{t.value}</TH>
              <TH className="w-[30%]">{t.status}</TH>
              <TH className="w-16">
                <span className="sr-only">{strings.users.columns.actions}</span>
              </TH>
            </THead>
            <tbody>
              {vars.length === 0 && added.length === 0 ? (
                <TR>
                  <TD colSpan={4} className="text-muted">
                    {t.empty}
                  </TD>
                </TR>
              ) : null}
              {vars.map((v) => {
                const isRemoved = removed.has(v.key);
                const shown = edits[v.key] ?? (v.secret ? (revealed[v.key] ?? "") : (v.value ?? ""));
                return (
                  <TR key={v.key}>
                    <TD
                      className={
                        isRemoved ? "font-mono font-medium text-muted line-through" : "font-mono font-medium"
                      }
                    >
                      {v.key}
                    </TD>
                    <TD>
                      <div className="flex items-center gap-2">
                        <Input
                          aria-label={`${t.value} ${v.key}`}
                          type={
                            v.secret && !(v.key in edits) && revealed[v.key] === undefined
                              ? "password"
                              : "text"
                          }
                          placeholder={
                            v.secret && revealed[v.key] === undefined ? strings.config.masked : undefined
                          }
                          value={shown}
                          readOnly={!canWrite || isRemoved}
                          onChange={(e) => setEdits((prev) => ({ ...prev, [v.key]: e.target.value }))}
                          className="font-mono"
                        />
                        {v.secret && canReveal && revealed[v.key] === undefined && !(v.key in edits) ? (
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t.reveal(v.key)}
                            onClick={() => reveal.mutate(v.key)}
                          >
                            <Eye aria-hidden />
                          </Button>
                        ) : null}
                      </div>
                    </TD>
                    <TD>
                      <VarStatus v={v} unsaved={changedKeys.includes(v.key)} />
                    </TD>
                    <TD className="py-0 text-right">
                      {canWrite ? (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={isRemoved ? `${strings.saveBar.discard} ${v.key}` : t.remove(v.key)}
                          onClick={() =>
                            setRemoved((prev) => {
                              const next = new Set(prev);
                              if (next.has(v.key)) next.delete(v.key);
                              else next.add(v.key);
                              return next;
                            })
                          }
                        >
                          {isRemoved ? <Undo2 aria-hidden /> : <Trash2 aria-hidden />}
                        </Button>
                      ) : null}
                    </TD>
                  </TR>
                );
              })}
              {added.map((d) => (
                <TR key={d.key}>
                  <TD className="font-mono font-medium">{d.key}</TD>
                  <TD>
                    <Input
                      aria-label={`${t.value} ${d.key}`}
                      type={d.secret ? "password" : "text"}
                      value={d.value ?? ""}
                      onChange={(e) =>
                        setAdded((prev) =>
                          prev.map((x) => (x.key === d.key ? { ...x, value: e.target.value } : x)),
                        )
                      }
                      className="font-mono"
                    />
                  </TD>
                  <TD className="text-sm font-semibold text-deploying-fg">{t.pendingSave}</TD>
                  <TD className="py-0 text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t.remove(d.key)}
                      onClick={() => setAdded((prev) => prev.filter((x) => x.key !== d.key))}
                    >
                      <Trash2 aria-hidden />
                    </Button>
                  </TD>
                </TR>
              ))}
            </tbody>
          </DataTable>
        )}
        {canWrite && env.isSuccess ? (
          <AddShared
            exists={(k) => vars.some((v) => v.key === k) || added.some((d) => d.key === k)}
            onAdd={(d) => setAdded((prev) => [...prev, d])}
          />
        ) : null}
      </Card>

      <SaveBar
        count={count}
        onDiscard={discard}
        extra={
          <label className="flex min-h-11 items-center gap-2 text-[15px]">
            <input
              type="checkbox"
              checked={restart}
              onChange={(e) => setRestart(e.target.checked)}
              className="size-[18px] accent-accent"
            />
            {t.restart}
          </label>
        }
      >
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          {save.isPending ? t.saving : t.saveApply}
        </Button>
      </SaveBar>
    </div>
  );
}

/** "Applied to 2 apps", "Overridden in 1 app · blog-api uses its own value", "Pending". */
function VarStatus({ v, unsaved }: { v: SharedVar; unsaved: boolean }) {
  const t = strings.sharedConfig;
  if (unsaved) return <span className="text-sm font-semibold text-deploying-fg">{t.pendingSave}</span>;
  if (v.apps.length === 0) return <span className="text-sm text-muted">{t.noApps}</span>;
  const applied = v.apps.filter((a) => a.status === "applied").length;
  const overridden = v.apps.filter((a) => a.status === "overridden").map((a) => a.app);
  const pending = v.apps.filter((a) => a.status === "pending").length;
  return (
    <span className="flex flex-col text-sm">
      {applied > 0 ? <span className="font-semibold text-running-fg">{t.applied(applied)}</span> : null}
      {pending > 0 ? <span className="font-semibold text-deploying-fg">{t.pending(pending)}</span> : null}
      {overridden.length > 0 ? (
        <>
          <span className="font-semibold text-muted">{t.overridden(overridden.length)}</span>
          <span className="text-[13px] text-muted">{t.usesOwn(overridden)}</span>
        </>
      ) : null}
    </span>
  );
}

function AddShared({ exists, onAdd }: { exists: (k: string) => boolean; onAdd: (d: Draft) => void }) {
  const t = strings.sharedConfig;
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [secret, setSecret] = useState(false);
  const keyError =
    key && !keyRe.test(key) ? t.keyInvalid : key && exists(key) ? strings.config.keyTaken : null;
  const errorId = useId();
  return (
    <form
      className="grid gap-2 border-t border-line bg-add-row-bg px-5 py-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto_auto] sm:items-start"
      onSubmit={(e) => {
        e.preventDefault();
        if (!key || keyError) return;
        onAdd({ key, value, secret: secret || secretRe.test(key), isNew: true });
        setKey("");
        setValue("");
        setSecret(false);
      }}
    >
      <div>
        <Input
          aria-label={t.newKey}
          placeholder="KEY"
          value={key}
          onChange={(e) => setKey(e.target.value.trim().toUpperCase())}
          aria-invalid={keyError ? true : undefined}
          aria-describedby={keyError ? errorId : undefined}
          className="font-mono"
        />
        {keyError ? (
          <p id={errorId} className="mt-1 text-sm text-failed">
            {keyError}
          </p>
        ) : null}
      </div>
      <Input
        aria-label={t.newValue}
        placeholder="value"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="font-mono"
      />
      <label className="flex min-h-11 items-center gap-2">
        <input
          type="checkbox"
          checked={secret}
          onChange={(e) => setSecret(e.target.checked)}
          className="size-[18px] accent-accent"
        />
        {t.secret}
      </label>
      <Button type="submit" variant="secondary" disabled={!key || !!keyError}>
        <Plus aria-hidden />
        {t.add}
      </Button>
    </form>
  );
}
