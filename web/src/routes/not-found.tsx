import { SearchX } from "lucide-react";
import { Link } from "react-router";
import { Card } from "~/components/app/card";
import { EmptyState } from "~/components/app/states";
import { buttonVariants } from "~/components/ui/button";
import { strings } from "~/lib/strings";

export default function NotFound() {
  return (
    <Card>
      <EmptyState
        icon={SearchX}
        title={strings.errors.notFoundTitle}
        action={
          <Link to="/" className={buttonVariants({ variant: "secondary" })}>
            {strings.errors.goHome}
          </Link>
        }
      >
        {strings.errors.notFoundBody}
      </EmptyState>
    </Card>
  );
}
