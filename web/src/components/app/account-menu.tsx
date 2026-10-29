import { ChevronsUpDown, LogOut, Settings, UserRound } from "lucide-react";
import { Link } from "react-router";
import { useCan } from "~/hooks/use-can";
import { useLogout } from "~/hooks/use-logout";
import type { Me } from "~/lib/api/gen/model";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";
import { MenuItem, MenuLinkItem, MenuPopup, MenuRoot, MenuSeparator, MenuTrigger } from "../ui/menu";
import { ThemeSwitcher } from "./theme-switcher";

export function Avatar({ name, className }: { name: string; className?: string }) {
  return (
    <span
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-full bg-nav-active font-bold text-accent uppercase",
        className,
      )}
      aria-hidden
    >
      {name.slice(0, 1)}
    </span>
  );
}

/** The account menu at the bottom of the sidebar (SPEC.md, 4.1). */
export function AccountMenu({ me }: { me: Me }) {
  const canSettings = useCan("settings.write");
  const logout = useLogout();
  return (
    <MenuRoot>
      <MenuTrigger
        aria-label={strings.account.menu}
        className="flex min-h-14 w-full cursor-pointer items-center gap-3 rounded-lg px-2 text-left hover:bg-subtle data-popup-open:bg-subtle"
      >
        <Avatar name={me.username} />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-semibold">{me.username}</span>
          <span className="block text-[13px] text-muted">{strings.roles[me.role]}</span>
        </span>
        <ChevronsUpDown className="size-4 text-muted" aria-hidden />
      </MenuTrigger>
      <MenuPopup side="top" align="start" className="w-64">
        <div className="px-3 pt-2 pb-2">
          <div className="text-[13px] text-muted">{strings.account.signedInAs}</div>
          <div className="truncate font-semibold">{me.username}</div>
        </div>
        <MenuSeparator />
        <ThemeSwitcher value={me.theme} />
        <MenuSeparator />
        <MenuLinkItem render={<Link to="/profile" />}>
          <UserRound aria-hidden />
          {strings.account.profile}
        </MenuLinkItem>
        {canSettings ? (
          <MenuLinkItem render={<Link to="/settings" />}>
            <Settings aria-hidden />
            {strings.account.settings}
          </MenuLinkItem>
        ) : null}
        <MenuSeparator />
        <MenuItem onClick={logout}>
          <LogOut aria-hidden />
          {strings.account.logOut}
        </MenuItem>
      </MenuPopup>
    </MenuRoot>
  );
}
