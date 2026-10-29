import { Menu as BaseMenu } from "@base-ui/react/menu";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "~/lib/utils";

export const MenuRoot = BaseMenu.Root;
export const MenuTrigger = BaseMenu.Trigger;
export const MenuRadioGroup = BaseMenu.RadioGroup;

export function MenuPopup({
  children,
  className,
  side = "bottom",
  align = "end",
}: {
  children: ReactNode;
  className?: string;
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  return (
    <BaseMenu.Portal>
      <BaseMenu.Positioner side={side} align={align} sideOffset={6} className="z-50 outline-none">
        <BaseMenu.Popup
          className={cn(
            "min-w-56 rounded-lg border border-line bg-surface p-1 text-ink shadow-lg outline-none",
            className,
          )}
        >
          {children}
        </BaseMenu.Popup>
      </BaseMenu.Positioner>
    </BaseMenu.Portal>
  );
}

const itemClass =
  "flex min-h-11 cursor-pointer items-center gap-2 rounded-md px-3 text-[15px] outline-none select-none data-highlighted:bg-subtle data-disabled:cursor-not-allowed data-disabled:opacity-50 [&_svg]:size-[18px] [&_svg]:text-muted";

export function MenuItem({
  className,
  danger,
  ...props
}: ComponentProps<typeof BaseMenu.Item> & { danger?: boolean }) {
  return (
    <BaseMenu.Item
      className={cn(itemClass, danger && "text-failed [&_svg]:text-failed", className)}
      {...props}
    />
  );
}

export function MenuLinkItem({ className, ...props }: ComponentProps<typeof BaseMenu.LinkItem>) {
  return <BaseMenu.LinkItem className={cn(itemClass, className)} {...props} />;
}

export function MenuRadioItem({ children, className, ...props }: ComponentProps<typeof BaseMenu.RadioItem>) {
  return (
    <BaseMenu.RadioItem className={cn(itemClass, className)} {...props}>
      <span className="flex size-[18px] items-center justify-center">
        <BaseMenu.RadioItemIndicator className="size-2 rounded-full bg-accent" />
      </span>
      {children}
    </BaseMenu.RadioItem>
  );
}

export function MenuLabel({ children }: { children: ReactNode }) {
  return <div className="px-3 pt-2 pb-1 text-[13px] font-semibold text-muted">{children}</div>;
}

export function MenuSeparator() {
  return <BaseMenu.Separator className="my-1 h-px bg-line" />;
}
