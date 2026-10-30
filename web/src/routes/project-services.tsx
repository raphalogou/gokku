import { useSuspenseQuery } from "@tanstack/react-query";
import { Database, FolderMinus } from "lucide-react";
import { Card, CardHeader } from "~/components/app/card";
import { ChipList } from "~/components/app/chip";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { useProjectContext, useRemoveMember } from "~/components/app/project-members";
import { RowMenu } from "~/components/app/row-menu";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button } from "~/components/ui/button";
import { MenuItem } from "~/components/ui/menu";
import { projectQuery } from "~/lib/queries";
import { datastoreName, serviceStatusKind } from "~/lib/status";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project-services";

export default function ProjectServices({ params }: Route.ComponentProps) {
  const { data: p } = useSuspenseQuery(projectQuery(params.slug));
  const { openAdd } = useProjectContext();
  const remove = useRemoveMember(p.slug);
  const t = strings.project;

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader
          title={t.servicesInProject}
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
          <DataTable label={t.servicesInProject} className="min-w-[600px]">
            <THead>
              <TH>{t.columns.service}</TH>
              <TH>{t.columns.type}</TH>
              <TH>{t.columns.status}</TH>
              <TH>{t.columns.usedBy}</TH>
              {openAdd ? (
                <TH className="w-16">
                  <span className="sr-only">{strings.users.columns.actions}</span>
                </TH>
              ) : null}
            </THead>
            <tbody>
              {p.services.map((s) => (
                <TR key={`${s.type}/${s.name}`}>
                  <TD className="font-semibold">{s.name}</TD>
                  <TD>{datastoreName(s.type)}</TD>
                  <TD>
                    <StatusPill kind={serviceStatusKind[s.status]} label={strings.serviceStatus[s.status]} />
                  </TD>
                  <TD>
                    <ChipList items={s.links} empty={t.none} />
                  </TD>
                  {openAdd ? (
                    <TD className="py-0 text-right">
                      <RowMenu name={s.name}>
                        <MenuItem
                          onClick={() => remove.mutate({ kind: "service", type: s.type, name: s.name })}
                        >
                          <FolderMinus aria-hidden />
                          {t.removeFromProject}
                        </MenuItem>
                      </RowMenu>
                    </TD>
                  ) : null}
                </TR>
              ))}
            </tbody>
          </DataTable>
        )}
      </Card>
      <p className="text-sm text-muted">{t.serviceNote}</p>
    </div>
  );
}
