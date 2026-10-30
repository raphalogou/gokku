import { useSuspenseQuery } from "@tanstack/react-query";
import { Boxes, FolderMinus } from "lucide-react";
import { Link } from "react-router";
import { Card, CardHeader } from "~/components/app/card";
import { ChipList } from "~/components/app/chip";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { LastDeploy } from "~/components/app/last-deploy";
import { useProjectContext, useRemoveMember } from "~/components/app/project-members";
import { RowMenu } from "~/components/app/row-menu";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button } from "~/components/ui/button";
import { MenuItem } from "~/components/ui/menu";
import { projectQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project-apps";

/** The apps of a project. Bulk actions arrive with project actions in M5. */
export default function ProjectApps({ params }: Route.ComponentProps) {
  const { data: p } = useSuspenseQuery(projectQuery(params.slug));
  const { openAdd } = useProjectContext();
  const remove = useRemoveMember(p.slug);
  const t = strings.project;

  return (
    <Card>
      <CardHeader
        title={t.appsInProject}
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
        <DataTable label={t.appsInProject} className="min-w-[720px]">
          <THead>
            <TH>{t.columns.app}</TH>
            <TH>{t.columns.status}</TH>
            <TH>{t.columns.domain}</TH>
            <TH>{t.columns.lastDeploy}</TH>
            <TH>{t.columns.uses}</TH>
            {openAdd ? (
              <TH className="w-16">
                <span className="sr-only">{strings.users.columns.actions}</span>
              </TH>
            ) : null}
          </THead>
          <tbody>
            {p.apps.map((a) => (
              <TR key={a.name}>
                <TD>
                  <Link to={`/apps/${a.name}`} className="font-semibold whitespace-nowrap hover:underline">
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
                  <LastDeploy app={a} />
                </TD>
                <TD>
                  <ChipList items={a.services.map((s) => s.name)} empty={t.none} />
                </TD>
                {openAdd ? (
                  <TD className="py-0 text-right">
                    <RowMenu name={a.name}>
                      <MenuItem onClick={() => remove.mutate({ kind: "app", name: a.name })}>
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
  );
}
