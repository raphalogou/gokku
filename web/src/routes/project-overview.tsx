import { useSuspenseQuery } from "@tanstack/react-query";
import { Boxes, Database } from "lucide-react";
import { Link } from "react-router";
import { Card, CardHeader } from "~/components/app/card";
import { Chip, ChipList } from "~/components/app/chip";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { DatastoreType } from "~/components/app/datastore-icon";
import { useProjectContext } from "~/components/app/project-members";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button } from "~/components/ui/button";
import type { ActivityEntry } from "~/lib/api/gen/model";
import { dateTime, timeAgo } from "~/lib/format";
import { projectQuery } from "~/lib/queries";
import { serviceStatusKind } from "~/lib/status";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project-overview";

/** Project overview (PRJ-3): counts, apps with their services, services with their apps,
 * and recent activity. */
export default function ProjectOverview({ params }: Route.ComponentProps) {
  const { data: p } = useSuspenseQuery(projectQuery(params.slug));
  const { openAdd } = useProjectContext();
  const t = strings.project;

  return (
    <div className="flex flex-col gap-6">
      <dl className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <Stat label={t.stats.apps} value={String(p.apps.length)} />
        <Stat label={t.stats.services} value={String(p.services.length)} />
        <Stat label={t.stats.domains} value={String(p.domain_count)} />
        <Stat
          label={t.stats.lastDeploy}
          value={p.last_deploy_at ? timeAgo(p.last_deploy_at) : strings.common.never}
          title={p.last_deploy_at ? dateTime(p.last_deploy_at) : undefined}
        />
      </dl>

      <div className="grid items-start gap-6 xl:grid-cols-2 [&>*]:min-w-0">
        <Card>
          <CardHeader
            title={t.appsTitle}
            actions={
              openAdd ? (
                <Button variant="secondary" onClick={() => openAdd("app")}>
                  {t.addApp}
                </Button>
              ) : null
            }
          />
          {p.apps.length === 0 ? (
            <EmptyState icon={Boxes} title={t.emptyAppsTitle}>
              {t.emptyAppsBody}
            </EmptyState>
          ) : (
            <DataTable label={t.appsTitle} className="min-w-[520px]">
              <THead>
                <TH>{t.columns.app}</TH>
                <TH>{t.columns.status}</TH>
                <TH>{t.columns.domain}</TH>
                <TH>{t.columns.uses}</TH>
              </THead>
              <tbody>
                {p.apps.map((a) => (
                  <TR key={a.name}>
                    <TD>
                      <Link
                        to={`/apps/${a.name}`}
                        className="font-semibold whitespace-nowrap hover:underline"
                      >
                        {a.name}
                      </Link>
                    </TD>
                    <TD>
                      <StatusPill kind={a.status} label={strings.appStatus[a.status]} />
                    </TD>
                    <TD className={a.domains[0] ? "whitespace-nowrap" : "whitespace-nowrap text-muted"}>
                      {a.domains[0] ?? t.noDomain}
                    </TD>
                    <TD>
                      <ChipList items={a.services.map((s) => s.name)} empty={t.none} />
                    </TD>
                  </TR>
                ))}
              </tbody>
            </DataTable>
          )}
        </Card>

        <Card>
          <CardHeader
            title={t.servicesTitle}
            actions={
              openAdd ? (
                <Button variant="secondary" onClick={() => openAdd("service")}>
                  {t.addService}
                </Button>
              ) : null
            }
          />
          {p.services.length === 0 ? (
            <EmptyState icon={Database} title={t.emptyServicesTitle}>
              {t.emptyServicesBody}
            </EmptyState>
          ) : (
            <DataTable label={t.servicesTitle} className="min-w-[480px]">
              <THead>
                <TH>{t.columns.service}</TH>
                <TH>{t.columns.status}</TH>
                <TH>{t.columns.usedBy}</TH>
              </THead>
              <tbody>
                {p.services.map((s) => (
                  <TR key={`${s.type}/${s.name}`}>
                    <TD>
                      <span className="font-semibold">{s.name}</span>
                      <DatastoreType type={s.type} className="ml-2 text-sm text-muted" />
                    </TD>
                    <TD>
                      <StatusPill
                        kind={serviceStatusKind[s.status]}
                        label={strings.serviceStatus[s.status]}
                      />
                    </TD>
                    <TD>
                      <ChipList items={s.links} empty={t.none} />
                    </TD>
                  </TR>
                ))}
              </tbody>
            </DataTable>
          )}
        </Card>
      </div>

      <Card>
        <CardHeader title={t.activityTitle} />
        {p.activity.length === 0 ? (
          <p className="px-5 py-6 text-muted">{t.activityEmpty}</p>
        ) : (
          <ul className="divide-y divide-line">
            {p.activity.map((e) => (
              <ActivityRow key={e.id} entry={e} />
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function Stat({ label, value, title }: { label: string; value: string; title?: string }) {
  return (
    <div className="rounded-xl border border-line bg-surface px-5 py-4">
      <dt className="text-[13px] font-semibold text-muted">{label}</dt>
      <dd className="mt-1 text-2xl font-bold" title={title}>
        {value}
      </dd>
    </div>
  );
}

function ActivityRow({ entry: e }: { entry: ActivityEntry }) {
  return (
    <li className="flex min-h-[52px] flex-wrap items-center justify-between gap-x-4 gap-y-1 px-5 py-3">
      <span className="min-w-0">
        <span className="font-semibold">{e.actor_name}</span>{" "}
        <span className="text-muted">{strings.activity.describe(e.action)}</span>{" "}
        {e.target_name ? <Chip>{e.target_name}</Chip> : null}
        {e.result === "fail" ? (
          <StatusPill kind="failed" label={strings.appStatus.failed} className="ml-2" />
        ) : null}
      </span>
      <time dateTime={e.ts} title={dateTime(e.ts)} className="text-sm text-muted">
        {timeAgo(e.ts)}
      </time>
    </li>
  );
}
