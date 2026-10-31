import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import {
  getApp,
  getAppConfig,
  getAppLogs,
  getJob,
  getMe,
  getProject,
  getProjectEnv,
  getServer,
  getSetupStatus,
  listApps,
  listAudit,
  listJobs,
  listMySessions,
  listPlugins,
  listProjects,
  listServices,
  listTokens,
  listUsers,
} from "./api/gen/client";
import type { GetAppLogsParams, ListAuditParams, ListJobsParams } from "./api/gen/model";

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
  jobs: () => ["jobs"] as const,
  jobList: (params: ListJobsParams) => ["jobs", "list", params] as const,
  job: (id: string) => ["jobs", id] as const,
  appLogs: (name: string, params: GetAppLogsParams) => ["logs", name, params] as const,
  appConfig: (name: string) => ["apps", name, "config"] as const,
  projectEnv: (slug: string) => ["projects", slug, "env"] as const,
  audit: () => ["audit"] as const,
  auditList: (params: ListAuditParams) => ["audit", params] as const,
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

export const jobQuery = (id: string) =>
  queryOptions({ queryKey: queryKeys.job(id), queryFn: ({ signal }) => getJob(id, { signal }) });

export const jobsQuery = (params: ListJobsParams) =>
  queryOptions({
    queryKey: queryKeys.jobList(params),
    queryFn: ({ signal }) => listJobs(params, { signal }),
  });

export const appLogsQuery = (name: string, params: GetAppLogsParams) =>
  queryOptions({
    queryKey: queryKeys.appLogs(name, params),
    queryFn: ({ signal }) => getAppLogs(name, params, { signal }),
    staleTime: 0,
  });

/** The audit log, one cursor page at a time (ACT-2). */
export const auditQuery = (params: Omit<ListAuditParams, "cursor">) =>
  infiniteQueryOptions({
    queryKey: queryKeys.auditList(params),
    queryFn: ({ signal, pageParam }) => listAudit({ ...params, cursor: pageParam }, { signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });

export const appConfigQuery = (name: string) =>
  queryOptions({
    queryKey: queryKeys.appConfig(name),
    queryFn: ({ signal }) => getAppConfig(name, { signal }),
  });

export const projectEnvQuery = (slug: string) =>
  queryOptions({
    queryKey: queryKeys.projectEnv(slug),
    queryFn: ({ signal }) => getProjectEnv(slug, { signal }),
  });
