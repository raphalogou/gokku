import { queryOptions } from "@tanstack/react-query";
import { getMe, getSetupStatus, listMySessions, listTokens, listUsers } from "./api/gen/client";

/** Every query key in the app comes from this factory (AGENTS.md). */
export const queryKeys = {
  setupStatus: () => ["setup-status"] as const,
  me: () => ["me"] as const,
  mySessions: () => ["me", "sessions"] as const,
  users: () => ["users"] as const,
  tokens: () => ["tokens"] as const,
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
