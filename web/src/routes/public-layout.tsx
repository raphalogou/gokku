import type { ReactNode } from "react";
import { Outlet } from "react-router";
import { BrandMark } from "~/components/app/brand";
import { strings } from "~/lib/strings";

export default function PublicLayout() {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center bg-bg px-4 py-12">
      <div className="mb-8 flex flex-col items-center gap-3 text-center">
        <BrandMark className="size-11" />
        <p className="text-[15px] text-muted">{strings.tagline}</p>
      </div>
      <Outlet />
    </main>
  );
}

export function PublicCard({
  title,
  intro,
  children,
}: {
  title: string;
  intro?: string;
  children: ReactNode;
}) {
  return (
    <div className="w-full max-w-[420px] rounded-xl border border-line bg-surface p-6 sm:p-8">
      <h1 className="text-[22px] leading-tight font-bold">{title}</h1>
      {intro ? <p className="mt-2 text-[15px] text-muted">{intro}</p> : null}
      <div className="mt-6">{children}</div>
    </div>
  );
}
