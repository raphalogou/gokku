import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Plus, Trash2, Undo2 } from "lucide-react";
import { useId, useState } from "react";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { useJobDock } from "~/components/app/job-dock";
import { SaveBar } from "~/components/app/save-bar";
import { Skeleton } from "~/components/app/states";
import { ValueField } from "~/components/app/value-field";
import { Button } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Input, Textarea } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { revealAppConfig, updateAppConfig } from "~/lib/api/gen/client";
import type { ConfigVar } from "~/lib/api/gen/model";
import { diffDotenv, formatDotenv, parseDotenv } from "~/lib/dotenv";
import { errorMessage } from "~/lib/forms";
import { appConfigQuery, appQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-config";

const keyRe = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** App config (CFG-1 to CFG-5): masked values with reveal, read-only managed variables,
 * batch edits with one optional restart, and editing as .env. */
export default function AppConfig({ params }: Route.ComponentProps) {
  const name = params.name;
  const config = useQuery(appConfigQuery(name));
  const { data: app } = useQuery(appQuery(name));
  const canWrite = useCan("app.config.write");
  const canReveal = useCan("app.secrets.reveal");
  const qc = useQueryClient();
  const dock = useJobDock();
  const t = strings.config;

  // Pending edits: new values by key, keys to remove, and keys added.
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [removed, setRemoved] = useState<Set<string>>(new Set());
  const [added, setAdded] = useState<string[]>([]);
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [restart, setRestart] = useState(false);
  const [envOpen, setEnvOpen] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const vars = config.data?.vars ?? [];
  const byKey = new Map(vars.map((v) => [v.key, v]));
  const changed = Object.keys(edits).filter((k) => byKey.get(k)?.value !== edits[k] || !byKey.has(k));
  const count = changed.length + removed.size;
  const discard = () => {
    setEdits({});
    setRemoved(new Set());
    setAdded([]);
    setSaveError(null);
  };

  const reveal = useMutation({
    mutationFn: (key: string) => revealAppConfig(name, { keys: [key] }),
    onSuccess: (r) => setRevealed((prev) => ({ ...prev, ...r.values })),
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const save = useMutation({
    mutationFn: () => {
      const set: Record<string, string> = {};
      for (const k of changed) set[k] = edits[k] ?? "";
      return updateAppConfig(name, { set, unset: [...removed], restart });
    },
    onSuccess: async (out) => {
      qc.setQueryData(queryKeys.appConfig(name), out.config);
      discard();
      setRevealed({});
      if (out.job_id) {
        notify(t.savedRestart);
        dock.track(out.job_id);
      } else {
        notify(t.saved);
      }
    },
    onError: (err) => setSaveError(errorMessage(err)),
  });

  const setValue = (key: string, value: string) => setEdits((prev) => ({ ...prev, [key]: value }));
  const toggleRemove = (key: string) =>
    setRemoved((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  return (
    <div className="flex max-w-5xl flex-col gap-4">
      <p className="max-w-3xl text-muted">{t.intro}</p>
      {!canWrite ? <Banner>{t.readOnly}</Banner> : null}
      {saveError ? <Banner variant="error">{saveError}</Banner> : null}
      <Card>
        <CardHeader
          title={t.title}
          actions={
            canWrite ? (
              <Button variant="secondary" onClick={() => setEnvOpen(true)} disabled={!config.isSuccess}>
                <FileText aria-hidden />
                {t.editEnv}
              </Button>
            ) : null
          }
        />
        {config.isPending ? (
          <div className="flex flex-col gap-3 p-5" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-11" />
            ))}
          </div>
        ) : config.isError ? (
          <div className="p-5">
            <Banner variant="error">{errorMessage(config.error)}</Banner>
          </div>
        ) : (
          <ul aria-label={t.title} className="divide-y divide-line">
            {vars.length === 0 && added.length === 0 ? (
              <li className="px-5 py-6 text-muted">{t.empty}</li>
            ) : null}
            {vars.map((v) => (
              <VarRow
                key={v.key}
                v={v}
                projectName={app?.project?.name}
                value={edits[v.key]}
                revealed={revealed[v.key]}
                removed={removed.has(v.key)}
                canWrite={canWrite}
                canReveal={canReveal}
                onChange={(value) => setValue(v.key, value)}
                onToggleRemove={() => toggleRemove(v.key)}
                onReveal={() => reveal.mutate(v.key)}
                readHidden={async () => (await revealAppConfig(name, { keys: [v.key] })).values[v.key]}
                onHide={() =>
                  setRevealed((prev) => {
                    const { [v.key]: _, ...rest } = prev;
                    return rest;
                  })
                }
              />
            ))}
            {added.map((k) => (
              <li
                key={k}
                className="grid gap-2 px-5 py-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_44px] sm:items-center"
              >
                <span className="font-mono font-medium break-all">{k}</span>
                <ValueField
                  name={k}
                  value={edits[k] ?? ""}
                  hidden={false}
                  secret={false}
                  readOnly={false}
                  onChange={(value) => setValue(k, value)}
                />
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t.remove(k)}
                  onClick={() => {
                    setAdded((prev) => prev.filter((x) => x !== k));
                    setEdits(({ [k]: _, ...rest }) => rest);
                  }}
                >
                  <Trash2 aria-hidden />
                </Button>
              </li>
            ))}
            {canWrite ? (
              <AddRow
                exists={(k) => byKey.has(k) || added.includes(k)}
                onAdd={(k, v) => {
                  setAdded((prev) => [...prev, k]);
                  setValue(k, v);
                }}
              />
            ) : null}
          </ul>
        )}
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
          {save.isPending ? t.saving : t.save}
        </Button>
      </SaveBar>

      {envOpen && config.data ? (
        <DotenvDialog
          vars={config.data.vars}
          onClose={() => setEnvOpen(false)}
          onApply={(set, unset) => {
            for (const [k, v] of Object.entries(set)) {
              if (!byKey.has(k)) setAdded((prev) => (prev.includes(k) ? prev : [...prev, k]));
              setValue(k, v);
            }
            setRemoved((prev) => new Set([...prev, ...unset]));
            setEnvOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}

function VarRow({
  v,
  projectName,
  value,
  revealed,
  removed,
  canWrite,
  canReveal,
  onChange,
  onToggleRemove,
  onReveal,
  onHide,
  readHidden,
}: {
  v: ConfigVar;
  projectName?: string;
  value?: string;
  revealed?: string;
  removed: boolean;
  canWrite: boolean;
  canReveal: boolean;
  onChange: (value: string) => void;
  onToggleRemove: () => void;
  onReveal: () => void;
  onHide: () => void;
  readHidden: () => Promise<string | undefined>;
}) {
  const t = strings.config;
  const managed = v.managed_by ?? null;
  const source = managed?.startsWith("service:")
    ? t.linkedTo(managed.slice(8))
    : managed
      ? t.inherited(projectName ?? managed.slice(8))
      : null;
  const shown = value ?? (v.masked ? (revealed ?? "") : (v.value ?? ""));
  const editable = canWrite && !managed && !removed;
  const hintId = useId();
  return (
    <li className="grid gap-2 px-5 py-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_44px] sm:items-center">
      <div className="min-w-0">
        <span
          className={
            removed
              ? "font-mono font-medium break-all text-muted line-through"
              : "font-mono font-medium break-all"
          }
        >
          {v.key}
        </span>
        {source ? (
          <span id={hintId} className="block text-[13px] text-muted">
            {source}
          </span>
        ) : null}
        {removed ? <span className="block text-[13px] text-failed-fg">{t.removed}</span> : null}
      </div>
      <div className="flex items-center gap-2">
        <ValueField
          name={v.key}
          value={shown}
          hidden={v.masked && revealed === undefined && value === undefined}
          secret={v.masked && value === undefined}
          readOnly={!editable}
          placeholder={v.masked && revealed === undefined ? t.masked : undefined}
          describedBy={source ? hintId : undefined}
          onChange={onChange}
          onReveal={canReveal ? onReveal : undefined}
          onHide={onHide}
          readHidden={canReveal ? readHidden : undefined}
        />
        {managed ? (
          <span className="rounded-full bg-chip px-2 py-0.5 text-[13px] font-semibold text-muted">
            {t.managed}
          </span>
        ) : null}
      </div>
      {canWrite && !managed ? (
        <Button
          variant="ghost"
          size="icon"
          aria-label={removed ? `${strings.saveBar.discard} ${v.key}` : t.remove(v.key)}
          onClick={onToggleRemove}
        >
          {removed ? <Undo2 aria-hidden /> : <Trash2 aria-hidden />}
        </Button>
      ) : (
        <span />
      )}
    </li>
  );
}

function AddRow({
  exists,
  onAdd,
}: {
  exists: (key: string) => boolean;
  onAdd: (key: string, value: string) => void;
}) {
  const t = strings.config;
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const keyError = key && !keyRe.test(key) ? t.keyInvalid : key && exists(key) ? t.keyTaken : null;
  const errorId = useId();
  return (
    <li className="bg-add-row-bg px-5 py-3">
      <form
        className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] sm:items-start"
        onSubmit={(e) => {
          e.preventDefault();
          if (!key || keyError) return;
          onAdd(key, value);
          setKey("");
          setValue("");
        }}
      >
        <div>
          <Input
            aria-label={t.newKey}
            placeholder="KEY"
            value={key}
            onChange={(e) => setKey(e.target.value.trim())}
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
        <Button type="submit" variant="secondary" disabled={!key || !!keyError}>
          <Plus aria-hidden />
          {t.add}
        </Button>
      </form>
    </li>
  );
}

/** Paste a .env, review the differences, then apply them to the form (CFG-5). */
function DotenvDialog({
  vars,
  onClose,
  onApply,
}: {
  vars: ConfigVar[];
  onClose: () => void;
  onApply: (set: Record<string, string>, unset: string[]) => void;
}) {
  const t = strings.config;
  const editable: Record<string, string> = {};
  const keep = new Set<string>();
  for (const v of vars) {
    if (v.masked || v.managed_by) keep.add(v.key);
    else editable[v.key] = v.value ?? "";
  }
  const [text, setText] = useState(() => formatDotenv(editable));
  const parsed = parseDotenv(text);
  const managed = new Set(vars.filter((v) => v.managed_by).map((v) => v.key));
  const next = Object.fromEntries(Object.entries(parsed.vars).filter(([k]) => !managed.has(k)));
  const diff = diffDotenv(editable, next, keep);
  const total = diff.added.length + diff.changed.length + diff.removed.length;
  const labelId = useId();
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t.envTitle}
      description={t.envIntro}
      className="max-w-2xl"
    >
      <label id={labelId} htmlFor={`${labelId}-text`} className="mb-1.5 block text-sm font-semibold">
        {t.envLabel}
      </label>
      <Textarea
        id={`${labelId}-text`}
        rows={12}
        value={text}
        onChange={(e) => setText(e.target.value)}
        spellCheck={false}
        className="font-mono text-[13px]"
      />
      {parsed.errors.length > 0 ? (
        <Banner variant="error" className="mt-3">
          {parsed.errors.join(". ")}
        </Banner>
      ) : (
        <p className="mt-3 text-sm text-muted" aria-live="polite">
          {total === 0
            ? t.envNothing
            : t.envReview(diff.added.length, diff.changed.length, diff.removed.length)}
        </p>
      )}
      <DialogActions>
        <Button variant="secondary" onClick={onClose}>
          {strings.common.cancel}
        </Button>
        <Button
          disabled={parsed.errors.length > 0 || total === 0}
          onClick={() => {
            const set: Record<string, string> = {};
            for (const k of [...diff.added, ...diff.changed]) set[k] = next[k] ?? "";
            onApply(set, diff.removed);
          }}
        >
          {t.envApply}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
