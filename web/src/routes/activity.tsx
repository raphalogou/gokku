import { ActivityLog } from "~/components/app/activity-log";
import { PageHeader } from "~/components/app/page-header";
import { strings } from "~/lib/strings";

/** Activity: the audit log of every change made through Gokku (ACT-2). */
export default function ActivityPage() {
  return (
    <>
      <PageHeader title={strings.activity.title} description={strings.activity.description} />
      <ActivityLog actions="header" />
    </>
  );
}
