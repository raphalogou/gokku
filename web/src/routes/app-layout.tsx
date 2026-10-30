import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { ShieldAlert, TriangleAlert } from "lucide-react";
import { useEffect } from "react";
import {
  isRouteErrorResponse,
  Link,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
  useRouteError,
} from "react-router";
import { ServerProblems } from "~/components/app/server-status";
import { Shell } from "~/components/app/shell";
import { EmptyState } from "~/components/app/states";
import { buttonVariants } from "~/components/ui/button";
import { useServerEvents } from "~/hooks/use-server-events";
import { useFollowSystemTheme } from "~/hooks/use-theme";
import { ApiError, setUnauthorizedHandler } from "~/lib/api/fetcher";
import { meQuery, setupStatusQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import { applyTheme } from "~/lib/theme";
import type { Route } from "./+types/app-layout";

/** Sends the visitor to setup or sign-in before any signed-in screen renders. */
export async function clientLoader({ request }: Route.ClientLoaderArgs) {
  const status = await queryClient.ensureQueryData(setupStatusQuery());
  if (status.setup_required) throw redirect("/setup");
  try {
    const me = await queryClient.ensureQueryData(meQuery());
    applyTheme(me.theme);
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      const url = new URL(request.url);
      throw redirect(`/login?next=${encodeURIComponent(url.pathname + url.search)}`);
    }
    throw err;
  }
  return null;
}

export default function AppLayout() {
  const { data: me } = useSuspenseQuery(meQuery());
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  useFollowSystemTheme(me.theme);

  // A 401 anywhere means the session ended: sign in again, then drop the cached data. The
  // handler runs once, and the cache is cleared only after the shell has unmounted, so its
  // queries cannot refetch and fire it again.
  useEffect(() => {
    setUnauthorizedHandler(async () => {
      setUnauthorizedHandler(undefined);
      await navigate(`/login?next=${encodeURIComponent(location.pathname + location.search)}`, {
        replace: true,
      });
      qc.clear();
    });
    return () => setUnauthorizedHandler(undefined);
  }, [qc, navigate, location.pathname, location.search]);

  const { connected } = useServerEvents();

  return (
    <Shell me={me}>
      <ServerProblems eventsConnected={connected} />
      <Outlet />
    </Shell>
  );
}

export function ErrorBoundary() {
  const error = useRouteError();
  const forbidden =
    (isRouteErrorResponse(error) && error.status === 403) ||
    (error instanceof ApiError && error.status === 403);
  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg px-4">
      <EmptyState
        icon={forbidden ? ShieldAlert : TriangleAlert}
        title={forbidden ? strings.errors.forbiddenTitle : strings.errors.loadFailed}
        action={
          <Link to="/" className={buttonVariants({ variant: "secondary" })}>
            {strings.errors.goHome}
          </Link>
        }
      >
        {forbidden
          ? strings.errors.forbiddenBody
          : error instanceof ApiError
            ? error.message
            : strings.errors.unexpected}
      </EmptyState>
    </main>
  );
}
