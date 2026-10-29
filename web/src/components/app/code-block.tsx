import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";

/** Monospace text with a copy button, for one-time links and tokens. */
export function CodeBlock({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access can be denied; the text is selectable as a fallback.
    }
  };
  return (
    <div className="flex items-stretch gap-2">
      <code className="min-w-0 flex-1 rounded-lg bg-code-bg px-3 py-2.5 font-mono text-[13px] break-all select-all">
        {value}
      </code>
      <Button variant="secondary" onClick={copy} aria-label={`${strings.common.copy}: ${label}`}>
        {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
        <span aria-live="polite">{copied ? strings.common.copied : strings.common.copy}</span>
      </Button>
    </div>
  );
}
