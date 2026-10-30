import { useSuspenseQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { StatusPill } from "~/components/app/status-pill";
import { dateTime, timeAgo } from "~/lib/format";
import { appQuery } from "~/lib/queries";
import { datastoreName, shortSha } from "~/lib/status";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-overview";

/** App overview, read-only in M2: latest deployment, processes, domains and services. */
export default function AppOverview({ params }: Route.ComponentProps) {
  const { data: app } = useSuspenseQuery(appQuery(params.name));
  const t = strings.app;

  return (
    <div className="grid items-start gap-6 xl:grid-cols-2 [&>*]:min-w-0">
      <Card>
        <CardHeader title={t.latestDeploy} />
        <CardBody>
          {app.deployed || app.revision ? (
            <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-3">
              {app.revision ? (
                <Row label={t.commit}>
                  <span className="font-mono" title={app.revision}>
                    {shortSha(app.revision)}
                  </span>
                </Row>
              ) : null}
              {app.source_image ? (
                <Row label={t.image}>
                  <span className="font-mono break-all">{app.source_image}</span>
                </Row>
              ) : null}
              {app.deploy_source ? (
                <Row label={t.source}>{t.sources[app.deploy_source] ?? app.deploy_source}</Row>
              ) : null}
              {app.builder ? <Row label={t.builder}>{t.builders[app.builder] ?? app.builder}</Row> : null}
              {app.deployed_at ? (
                <Row label={t.deployed}>
                  <time dateTime={app.deployed_at} title={dateTime(app.deployed_at)}>
                    {timeAgo(app.deployed_at)}
                  </time>
                </Row>
              ) : null}
              {app.created_at ? (
                <Row label={t.created}>
                  <time dateTime={app.created_at}>{dateTime(app.created_at)}</time>
                </Row>
              ) : null}
            </dl>
          ) : (
            <p className="text-muted">{t.notDeployed}</p>
          )}
        </CardBody>
      </Card>

      <Card>
        <CardHeader title={t.processes} />
        {app.processes.length === 0 ? (
          <CardBody>
            <p className="text-muted">{t.noProcesses}</p>
          </CardBody>
        ) : (
          <DataTable label={t.processes}>
            <THead>
              <TH>{t.processColumns.type}</TH>
              <TH>{t.processColumns.instances}</TH>
              <TH>{t.processColumns.status}</TH>
            </THead>
            <tbody>
              {app.processes.map((p) => (
                <TR key={p.type}>
                  <TD className="font-mono font-medium">{p.type}</TD>
                  <TD>{p.count}</TD>
                  <TD>
                    <StatusPill
                      kind={p.running === p.count ? "running" : p.running === 0 ? "stopped" : "deploying"}
                      label={t.processRunning(p.running, p.count)}
                    />
                  </TD>
                </TR>
              ))}
            </tbody>
          </DataTable>
        )}
      </Card>

      <Card>
        <CardHeader title={t.domainsTitle} />
        {app.domains.length === 0 ? (
          <CardBody>
            <p className="text-muted">{t.noDomains}</p>
          </CardBody>
        ) : (
          <ul className="divide-y divide-line">
            {app.domains.map((d) => (
              <li key={d} className="flex min-h-[52px] items-center px-5 py-2">
                <a
                  href={`http://${d}`}
                  target="_blank"
                  rel="noreferrer"
                  className="break-all hover:underline"
                >
                  {d}
                </a>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card>
        <CardHeader title={t.servicesTitle} />
        {app.services.length === 0 ? (
          <CardBody>
            <p className="text-muted">{t.noServices}</p>
          </CardBody>
        ) : (
          <ul className="divide-y divide-line">
            {app.services.map((s) => (
              <li key={`${s.type}/${s.name}`} className="flex min-h-[52px] items-center gap-3 px-5 py-2">
                <span className="font-semibold">{s.name}</span>
                <span className="text-sm text-muted">{datastoreName(s.type)}</span>
                <span className="ml-auto text-sm font-semibold text-running-fg">{t.linked}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </>
  );
}
