import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  children?: ReactNode;
  className?: string;
};

/** A modal dialog. Focus is trapped while open and returns to the trigger on close. */
export function Dialog({ open, onOpenChange, title, description, children, className }: DialogProps) {
  return (
    <BaseDialog.Root open={open} onOpenChange={onOpenChange}>
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-40 bg-black/40" />
        <BaseDialog.Popup
          className={cn(
            "fixed top-1/2 left-1/2 z-50 max-h-[calc(100dvh-32px)] w-[calc(100vw-32px)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto",
            "rounded-2xl border border-line bg-surface p-6 text-ink shadow-xl outline-none",
            className,
          )}
        >
          <div className="mb-4 flex items-start justify-between gap-4">
            <BaseDialog.Title className="text-lg font-bold">{title}</BaseDialog.Title>
            <BaseDialog.Close
              aria-label={strings.common.close}
              className="-mt-2 -mr-2 flex size-11 shrink-0 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle"
            >
              <X className="size-[18px]" aria-hidden />
            </BaseDialog.Close>
          </div>
          {description ? (
            <BaseDialog.Description className="mb-5 text-[15px] text-muted">
              {description}
            </BaseDialog.Description>
          ) : null}
          {children}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}

export function DialogActions({ children }: { children: ReactNode }) {
  return <div className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">{children}</div>;
}
