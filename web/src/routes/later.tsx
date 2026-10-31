import { Construction } from "lucide-react";
import { useMatches } from "react-router";
import { Card } from "~/components/app/card";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState } from "~/components/app/states";
import { strings } from "~/lib/strings";

const titles: Record<string, string> = {
  settings: strings.account.settings,
};

/** Placeholder for screens of later milestones. */
export default function Later() {
  const id = useMatches().at(-1)?.id ?? "";
  const title = titles[id] ?? "";
  return (
    <>
      <PageHeader title={title} />
      <Card>
        <EmptyState icon={Construction} title={strings.later.title(title)}>
          {strings.later.body}
        </EmptyState>
      </Card>
    </>
  );
}
