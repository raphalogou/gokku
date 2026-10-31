import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { Console, ConsoleControls } from "~/components/app/console";
import { SearchInput } from "~/components/ui/search-input";
import { Select } from "~/components/ui/select";
import { useConsoleBuffer } from "~/hooks/use-console-buffer";
import { toLogConsole, useLogStream } from "~/hooks/use-log-stream";
import type { GetAppLogsLines } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { appLogsQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-logs";

const sizes: GetAppLogsLines[] = [100, 500, 1000];
const all = "*";

/** App logs: tail with process and text filters (LOG-1), live follow with pause (LOG-2) and
 * download (LOG-3). */
export default function AppLogs({ params }: Route.ComponentProps) {
  const name = params.name;
  const [process, setProcess] = useState(all);
  const [lines, setLines] = useState<GetAppLogsLines>(100);
  const [filter, setFilter] = useState("");
  const buffer = useConsoleBuffer();
  const t = strings.logs;

  const proc = process === all ? undefined : process;
  const tail = useQuery(appLogsQuery(name, proc ? { lines, process: proc } : { lines }));
  const { reset } = buffer;
  useEffect(() => {
    if (tail.data) reset(tail.data.lines.map(toLogConsole));
  }, [tail.data, reset]);
  const stream = useLogStream(name, proc, tail.isSuccess, buffer.push);

  const q = filter.trim().toLowerCase();
  const visible = q
    ? buffer.lines.filter((l) => `${l.label ?? ""} ${l.level} ${l.text}`.toLowerCase().includes(q))
    : buffer.lines;
  const processes = tail.data?.processes ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Select
          aria-label={t.process}
          value={process}
          onValueChange={setProcess}
          options={[{ value: all, label: t.allProcesses }, ...processes.map((p) => ({ value: p, label: p }))]}
          className="w-full sm:w-[180px]"
        />
        <SearchInput label={t.filter} value={filter} onChange={setFilter} />
        <Select
          aria-label={t.lines}
          value={String(lines)}
          onValueChange={(v) => setLines(Number(v) as GetAppLogsLines)}
          options={sizes.map((n) => ({ value: String(n), label: t.last(n) }))}
          className="w-full sm:w-[180px]"
        />
      </div>
      {tail.isError ? <Banner variant="error">{errorMessage(tail.error)}</Banner> : null}
      {stream === "error" ? <Banner variant="error">{t.streamError}</Banner> : null}
      <Card>
        <CardHeader title={t.title} />
        <div className="flex flex-col gap-3 p-4">
          <Console
            lines={visible}
            label={t.title}
            follow={!buffer.paused}
            empty={tail.isPending ? strings.common.loading : q ? t.noMatch : t.empty}
          />
          <ConsoleControls
            lines={visible}
            paused={buffer.paused}
            waiting={buffer.waiting}
            onPausedChange={buffer.setPaused}
            live={stream === "live" || stream === "connecting"}
            filename={`${name}${proc ? `-${proc}` : ""}.log`}
          />
        </div>
      </Card>
    </div>
  );
}
