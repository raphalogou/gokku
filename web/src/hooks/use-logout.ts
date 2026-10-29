import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { setUnauthorizedHandler } from "~/lib/api/fetcher";
import { logout } from "~/lib/api/gen/client";

export function useLogout() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return async () => {
    // Signing out on purpose is not a session expiry: the 401s that follow must not redirect.
    setUnauthorizedHandler(undefined);
    try {
      await logout();
    } finally {
      // Leave the shell before clearing the cache, so mounted queries do not refetch.
      await navigate("/login", { replace: true });
      qc.clear();
    }
  };
}
