import { useState } from "react";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";
import { Dialog, DialogActions } from "../ui/dialog";
import { Field } from "../ui/field";
import { Input } from "../ui/input";
import { Banner } from "./banner";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  /** What the user must type, usually the name of the thing being destroyed. */
  confirmValue: string;
  confirmLabel: string;
  onConfirm: () => void;
  pending?: boolean;
  error?: string;
};

/** Destructive actions require typing the name (DESIGN.md, 5.4). */
export function TypedConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmValue,
  confirmLabel,
  onConfirm,
  pending,
  error,
}: Props) {
  const [typed, setTyped] = useState("");
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setTyped("");
        onOpenChange(o);
      }}
      title={title}
      description={description}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (typed === confirmValue) onConfirm();
        }}
      >
        {error ? (
          <Banner variant="error" className="mb-4">
            {error}
          </Banner>
        ) : null}
        <Field label={strings.confirm.typeToConfirm(confirmValue)}>
          <Input
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </Field>
        <DialogActions>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {strings.common.cancel}
          </Button>
          <Button type="submit" variant="danger" disabled={typed !== confirmValue || pending}>
            {confirmLabel}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
