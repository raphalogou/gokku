import { queryOptions } from "@tanstack/react-query";
import {
  getApp,
  getMe,
  getProject,
  getServer,
  getSetupStatus,
  listApps,
  listMySessions,
  listPlugins,
  listProjects,
  listServices,
  listTokens,
  listUsers,
} from "./api/gen/client";

/** Every query key in the app comes from this factory (AGENTS.md). */
export const queryKeys = {
  setupStatus: () => ["setup-status"] as const,
  me: () => ["me"] as const,
  mySessions: () => ["me", "sessions"] as const,
  users: () => ["users"] as const,
  tokens: () => ["tokens"] as const,
  server: () => ["server"] as const,
  plugins: () => ["plugins"] as const,
  projects: () => ["projects"] as const,
  project: (slug: string) => ["projects", slug] as const,
  apps: () => ["apps"] as const,
  app: (name: string) => ["apps", name] as const,
  services: () => ["services"] as const,
};

export const setupStatusQuery = () =>
  queryOptions({ queryKey: queryKeys.setupStatus(), queryFn: ({ signal }) => getSetupStatus({ signal }) });

export const meQuery = () =>
  queryOptions({ queryKey: queryKeys.me(), queryFn: ({ signal }) => getMe({ signal }) });

export const mySessionsQuery = () =>
  queryOptions({ queryKey: queryKeys.mySessions(), queryFn: ({ signal }) => listMySessions({ signal }) });

export const usersQuery = () =>
  queryOptions({ queryKey: queryKeys.users(), queryFn: ({ signal }) => listUsers({ signal }) });

export const tokensQuery = () =>
  queryOptions({ queryKey: queryKeys.tokens(), queryFn: ({ signal }) => listTokens({ signal }) });

export const serverQuery = () =>
  queryOptions({ queryKey: queryKeys.server(), queryFn: ({ signal }) => getServer({ signal }) });

export const pluginsQuery = () =>
  queryOptions({ queryKey: queryKeys.plugins(), queryFn: ({ signal }) => listPlugins({ signal }) });

export const projectsQuery = () =>
  queryOptions({ queryKey: queryKeys.projects(), queryFn: ({ signal }) => listProjects({ signal }) });

export const projectQuery = (slug: string) =>
  queryOptions({ queryKey: queryKeys.project(slug), queryFn: ({ signal }) => getProject(slug, { signal }) });

export const appsQuery = () =>
  queryOptions({ queryKey: queryKeys.apps(), queryFn: ({ signal }) => listApps({ signal }) });

export const appQuery = (name: string) =>
  queryOptions({ queryKey: queryKeys.app(name), queryFn: ({ signal }) => getApp(name, { signal }) });

export const servicesQuery = () =>
  queryOptions({ queryKey: queryKeys.services(), queryFn: ({ signal }) => listServices({ signal }) });
