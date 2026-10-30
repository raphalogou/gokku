import { Link } from "react-router";

export type Crumb = { to?: string; label: string };

/** "Projects / Blog platform": the last crumb is the current page. */
export function Breadcrumbs({ items }: { items: Crumb[] }) {
  return (
    <nav aria-label="Breadcrumb" className="mb-2 text-sm text-muted">
      <ol className="flex flex-wrap items-center gap-1.5">
        {items.map((c, i) => (
          <li key={c.to ?? c.label} className="flex items-center gap-1.5">
            {i > 0 ? <span aria-hidden>/</span> : null}
            {c.to ? (
              <Link to={c.to} className="inline-flex min-h-11 items-center font-semibold hover:text-ink">
                {c.label}
              </Link>
            ) : (
              <span aria-current="page">{c.label}</span>
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
}
