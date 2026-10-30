import { NavLink } from "react-router";
import { cn } from "~/lib/utils";

export type RouteTab = { to: string; label: string; end?: boolean };

/** Tabs that are links; the active one has a 2 px accent underline (DESIGN.md, 5.4). */
export function RouteTabs({ label, tabs }: { label: string; tabs: RouteTab[] }) {
  return (
    <nav aria-label={label} className="relative mb-6 overflow-x-auto border-b border-line">
      <ul className="flex min-w-max gap-1">
        {tabs.map((t) => (
          <li key={t.to}>
            <NavLink
              to={t.to}
              end={t.end}
              className={({ isActive }) =>
                cn(
                  "-mb-px flex min-h-11 items-center border-b-2 px-3 font-semibold whitespace-nowrap",
                  isActive ? "border-accent text-accent" : "border-transparent text-muted hover:text-ink",
                )
              }
            >
              {t.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
