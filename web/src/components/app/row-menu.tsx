import { Ellipsis } from "lucide-react";
import type { ReactNode } from "react";
import { strings } from "~/lib/strings";
import { MenuPopup, MenuRoot, MenuTrigger } from "../ui/menu";

/** The "..." button at the end of a table row. */
export function RowMenu({ name, children }: { name: string; children: ReactNode }) {
  return (
    <MenuRoot>
      <MenuTrigger
        aria-label={strings.common.moreActions(name)}
        className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle data-popup-open:bg-subtle"
      >
        <Ellipsis className="size-[18px]" aria-hidden />
      </MenuTrigger>
      <MenuPopup>{children}</MenuPopup>
    </MenuRoot>
  );
}
