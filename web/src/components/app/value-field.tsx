import { Check, Copy, Eye, EyeOff } from "lucide-react";
import { useState } from "react";
import { strings } from "~/lib/strings";
import { InputAddon, InputGroup, InputGroupInput } from "../ui/input";
import { notify } from "../ui/toast";

type Props = {
  /** The variable name, for the field's and buttons' labels. */
  name: string;
  /** The value to show: typed, revealed or plain. Empty while a masked value is hidden. */
  value: string;
  /** The value is secret and not revealed: the field shows dots and the placeholder. */
  hidden: boolean;
  /** The value is secret, whether revealed or not, so it can be hidden again. */
  secret: boolean;
  readOnly: boolean;
  placeholder?: string;
  describedBy?: string;
  onChange: (value: string) => void;
  /** Reveal and hide. Leave reveal out when the role cannot see secrets (CFG-2). */
  onReveal?: () => void;
  onHide?: () => void;
  /** Reads a hidden value for copying without showing it; audited like a reveal. */
  readHidden?: () => Promise<string | undefined>;
};

/** A config value with reveal and copy buttons inside the field. */
export function ValueField({
  name,
  value,
  hidden,
  secret,
  readOnly,
  placeholder,
  describedBy,
  onChange,
  onReveal,
  onHide,
  readHidden,
}: Props) {
  const [copied, setCopied] = useState(false);
  const t = strings.config;
  const canCopy = !hidden || !!readHidden;

  const copy = async () => {
    try {
      const text = hidden && readHidden ? await readHidden() : value;
      if (text === undefined) return;
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      notify(t.copyFailed, "error");
    }
  };

  return (
    <InputGroup readOnly={readOnly}>
      <InputGroupInput
        aria-label={`${t.value} ${name}`}
        aria-describedby={describedBy}
        type={hidden ? "password" : "text"}
        placeholder={placeholder}
        value={value}
        readOnly={readOnly}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
        autoComplete="off"
        className="font-mono"
      />
      {secret && onReveal ? (
        <InputAddon aria-label={hidden ? t.reveal(name) : t.hide(name)} onClick={hidden ? onReveal : onHide}>
          {hidden ? <Eye aria-hidden /> : <EyeOff aria-hidden />}
        </InputAddon>
      ) : null}
      {canCopy ? (
        <InputAddon aria-label={t.copy(name)} onClick={() => void copy()}>
          {copied ? <Check aria-hidden className="text-running-fg" /> : <Copy aria-hidden />}
          <span className="sr-only" aria-live="polite">
            {copied ? strings.common.copied : ""}
          </span>
        </InputAddon>
      ) : null}
    </InputGroup>
  );
}
