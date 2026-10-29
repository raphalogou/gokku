import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api/fetcher";

/** The one query client of the SPA. Route loaders prefetch into it. */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Retry only errors that can pass: network failures and server errors.
      retry: (count, err) =>
        count < 2 && (!(err instanceof ApiError) || err.status === 0 || err.status >= 500),
      refetchOnWindowFocus: true,
    },
  },
});
