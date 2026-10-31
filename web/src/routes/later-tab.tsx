import { Construction } from "lucide-react";
import { useMatches } from "react-router";
import { Card } from "~/components/app/card";
import { EmptyState } from "~/components/app/states";
import { strings } from "~/lib/strings";

const titles: Record<string, string> = {
  "app-deploy": strings.app.tabs.deploy,
};

/** Placeholder for a project or app tab of a later milestone. */
export default function LaterTab() {
  const id = useMatches().at(-1)?.id ?? "";
  return (
    <Card>
      <EmptyState icon={Construction} title={strings.later.title(titles[id] ?? "")}>
        {strings.later.body}
      </EmptyState>
    </Card>
  );
}
