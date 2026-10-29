import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { notify } from "~/components/ui/toast";
import { updateMe } from "~/lib/api/gen/client";
import type { Me, Theme } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { queryKeys } from "~/lib/queries";
import { applyTheme } from "~/lib/theme";

/** Changes the theme at once, then saves it to the account (USR-4). */
export function useSetTheme() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (theme: Theme) => updateMe({ theme }),
    onMutate: (theme) => {
      const previous = qc.getQueryData<Me>(queryKeys.me());
      applyTheme(theme);
      if (previous) qc.setQueryData<Me>(queryKeys.me(), { ...previous, theme });
      return { previous };
    },
    onError: (err, _theme, ctx) => {
      if (ctx?.previous) {
        applyTheme(ctx.previous.theme);
        qc.setQueryData(queryKeys.me(), ctx.previous);
      }
      notify(errorMessage(err), "error");
    },
    onSuccess: (me) => qc.setQueryData(queryKeys.me(), me),
  });
}

/** Follows the operating system's color scheme while the preference is "system". */
export function useFollowSystemTheme(theme: Theme | undefined) {
  useEffect(() => {
    if (theme !== "system") return;
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => applyTheme("system");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);
}
