import type { AppSummary } from "~/lib/api/gen/model";
import { dateTime, timeAgo } from "~/lib/format";
import { shortSha } from "~/lib/status";
import { strings } from "~/lib/strings";

/** "a3f9c21 · 2 hours ago", "Building now" while deploying, "Last deploy failed", or
 * "Never deployed". */
export function LastDeploy({ app }: { app: AppSummary }) {
  if (app.status === "deploying") {
    return (
      <span className="flex flex-col">
        {app.revision ? <span className="font-mono text-[13px]">{shortSha(app.revision)}</span> : null}
        <span className="text-sm text-deploying-fg">{strings.apps.buildingNow}</span>
      </span>
    );
  }
  if (app.status === "failed") {
    return (
      <span className="flex flex-col">
        {app.revision ? <span className="font-mono text-[13px]">{shortSha(app.revision)}</span> : null}
        <span className="text-sm text-failed-fg">{strings.apps.deployFailed}</span>
      </span>
    );
  }
  if (!app.deployed_at) return <span className="text-muted">{strings.apps.never}</span>;
  return (
    <span className="flex flex-col">
      {app.revision ? <span className="font-mono text-[13px]">{shortSha(app.revision)}</span> : null}
      <time dateTime={app.deployed_at} title={dateTime(app.deployed_at)} className="text-sm text-muted">
        {timeAgo(app.deployed_at)}
      </time>
    </span>
  );
}
