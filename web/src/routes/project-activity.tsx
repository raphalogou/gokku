import { ActivityLog } from "~/components/app/activity-log";
import type { Route } from "./+types/project-activity";

/** The project's activity tab: the audit log filtered to this project. */
export default function ProjectActivity({ params }: Route.ComponentProps) {
  return <ActivityLog project={params.slug} actions="header" />;
}
