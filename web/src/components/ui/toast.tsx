import { Toast } from "@base-ui/react/toast";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { strings } from "~/lib/strings";

export const toastManager = Toast.createToastManager();

/** Shows a short confirmation. Screen readers announce it through the toast live region. */
export function notify(title: string, type: "success" | "error" = "success") {
  toastManager.add({ title, type, timeout: type === "error" ? 8000 : 4000 });
}

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <Toast.Provider toastManager={toastManager}>
      {children}
      <Toast.Portal>
        <Toast.Viewport className="fixed right-4 bottom-4 z-50 flex w-[calc(100vw-32px)] max-w-sm flex-col gap-2 nav:bottom-6">
          <ToastList />
        </Toast.Viewport>
      </Toast.Portal>
    </Toast.Provider>
  );
}

function ToastList() {
  const { toasts } = Toast.useToastManager();
  return toasts.map((t) => (
    <Toast.Root
      key={t.id}
      toast={t}
      className="flex items-start gap-3 rounded-xl border border-line bg-surface p-4 text-ink shadow-lg data-[type=error]:border-failed"
    >
      <Toast.Title className="flex-1 text-[15px]" />
      <Toast.Close
        aria-label={strings.common.close}
        className="-m-2 flex size-9 cursor-pointer items-center justify-center rounded-md text-muted hover:bg-subtle"
      >
        <X className="size-4" aria-hidden />
      </Toast.Close>
    </Toast.Root>
  ));
}
