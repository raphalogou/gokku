import { useQuery } from "@tanstack/react-query";
import type { Role } from "~/lib/api/gen/model";
import { meQuery } from "~/lib/queries";

// The minimum roles of the actions the UI checks, mirroring internal/authz/table.go.
// This only hides controls; the server enforces every permission (PERM-2).
const minRole = {
  "user.manage": "admin",
  "token.manage": "admin",
  "settings.write": "admin",
  "server.rescan": "admin",
  "project.create": "admin",
  "project.members.write": "admin",
  "app.operate": "operator",
  "job.cancel": "operator",
  "audit.export": "admin",
  "app.config.write": "operator",
  "app.domains.write": "operator",
  "app.ssl.write": "operator",
  "app.scale": "operator",
  "app.create": "admin",
  "app.rename": "admin",
  "app.move": "admin",
  "app.destroy": "admin",
  "project.update": "admin",
  "project.delete": "admin",
  "app.secrets.reveal": "operator",
  "project.env.write": "admin",
} as const satisfies Record<string, Role>;

const rank: Record<Role, number> = { viewer: 1, operator: 2, admin: 3 };

export type UIAction = keyof typeof minRole;

export function useCan(action: UIAction): boolean {
  const { data: me } = useQuery(meQuery());
  return !!me && rank[me.role] >= rank[minRole[action]];
}
