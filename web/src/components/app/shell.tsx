import {
  Activity,
  Boxes,
  Database,
  Ellipsis,
  FolderKanban,
  LogOut,
  type LucideIcon,
  Puzzle,
  Server,
  Settings,
  UserRound,
  Users,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import { Link, NavLink } from "react-router";
import { useCan } from "~/hooks/use-can";
import { useLogout } from "~/hooks/use-logout";
import type { Me } from "~/lib/api/gen/model";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";
import { Dialog } from "../ui/dialog";
import { AccountMenu, Avatar } from "./account-menu";
import { Brand } from "./brand";
import { ThemeSwitcher } from "./theme-switcher";

type NavItem = { to: string; label: string; icon: LucideIcon; adminOnly?: boolean };

const mainNav: NavItem[] = [
  { to: "/projects", label: strings.nav.projects, icon: FolderKanban },
  { to: "/apps", label: strings.nav.apps, icon: Boxes },
  { to: "/services", label: strings.nav.services, icon: Database },
  { to: "/plugins", label: strings.nav.plugins, icon: Puzzle },
  { to: "/activity", label: strings.nav.activity, icon: Activity },
  { to: "/users", label: strings.nav.users, icon: Users, adminOnly: true },
];

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "flex min-h-11 items-center gap-3 rounded-lg px-3 font-semibold",
    isActive ? "bg-nav-active text-accent" : "text-ink hover:bg-subtle",
  );

/** The signed-in layout: sidebar on desktop, top bar and bottom tabs below 900 px. */
export function Shell({ me, children }: { me: Me; children: ReactNode }) {
  const isAdmin = useCan("user.manage");
  const nav = mainNav.filter((n) => !n.adminOnly || isAdmin);
  return (
    <div className="min-h-dvh bg-bg nav:flex">
      <aside className="sticky top-0 hidden h-dvh w-[232px] shrink-0 flex-col border-r border-line bg-surface p-4 nav:flex">
        <div className="mb-6 px-2 pt-1">
          <Brand />
        </div>
        <nav aria-label={strings.nav.label} className="flex flex-col gap-1">
          {nav.map(({ to, label, icon: Icon }) => (
            <NavLink key={to} to={to} className={navLinkClass}>
              <Icon className="size-[18px]" aria-hidden />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex flex-col gap-3">
          <div className="flex items-center gap-2 rounded-lg bg-chip px-3 py-2 text-[13px]">
            <Server className="size-4 text-muted" aria-hidden />
            <span className="text-muted">{strings.nav.server}</span>
            <span className="ml-auto truncate font-mono">{location.hostname}</span>
          </div>
          <AccountMenu me={me} />
        </div>
      </aside>

      <MobileTopBar me={me} />
      <main className="min-w-0 flex-1 px-4 pt-6 pb-28 nav:px-10 nav:pt-8 nav:pb-12">{children}</main>
      <MobileTabBar me={me} />
    </div>
  );
}

function MobileTopBar({ me }: { me: Me }) {
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-line bg-surface px-4 nav:hidden">
      <Brand />
      <Link
        to="/profile"
        aria-label={strings.account.profile}
        className="flex size-11 items-center justify-center"
      >
        <Avatar name={me.username} className="size-8 text-sm" />
      </Link>
    </header>
  );
}

const tabs: NavItem[] = mainNav.filter((n) =>
  ["/projects", "/apps", "/services", "/activity"].includes(n.to),
);

function MobileTabBar({ me }: { me: Me }) {
  const [moreOpen, setMoreOpen] = useState(false);
  const isAdmin = useCan("user.manage");
  const logout = useLogout();
  const tabClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 text-[12px] font-semibold",
      isActive ? "text-accent" : "text-muted",
    );
  const more: NavItem[] = [
    { to: "/plugins", label: strings.nav.plugins, icon: Puzzle },
    ...(isAdmin ? [{ to: "/users", label: strings.nav.users, icon: Users }] : []),
    { to: "/profile", label: strings.account.profile, icon: UserRound },
    ...(isAdmin ? [{ to: "/settings", label: strings.account.settings, icon: Settings }] : []),
  ];
  return (
    <>
      <nav
        aria-label={strings.nav.label}
        className="fixed inset-x-0 bottom-0 z-30 flex border-t border-line bg-surface pb-[env(safe-area-inset-bottom)] nav:hidden"
      >
        {tabs.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} className={tabClass}>
            <Icon className="size-5" aria-hidden />
            {label}
          </NavLink>
        ))}
        <button type="button" onClick={() => setMoreOpen(true)} className={tabClass({ isActive: false })}>
          <Ellipsis className="size-5" aria-hidden />
          {strings.nav.more}
        </button>
      </nav>
      <Dialog open={moreOpen} onOpenChange={setMoreOpen} title={strings.nav.more}>
        <div className="flex flex-col gap-1">
          {more.map(({ to, label, icon: Icon }) => (
            <NavLink key={to} to={to} className={navLinkClass} onClick={() => setMoreOpen(false)}>
              <Icon className="size-[18px]" aria-hidden />
              {label}
            </NavLink>
          ))}
          <div className="my-2 border-t border-line" />
          <ThemeSwitcher value={me.theme} />
          <div className="my-2 border-t border-line" />
          <button type="button" onClick={logout} className={navLinkClass({ isActive: false })}>
            <LogOut className="size-[18px]" aria-hidden />
            {strings.account.logOut}
          </button>
        </div>
      </Dialog>
    </>
  );
}
