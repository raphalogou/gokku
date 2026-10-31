/** Parsing and diffing .env text for the "Edit as .env" dialog (CFG-5). */

export type EnvParse = { vars: Record<string, string>; errors: string[] };

const keyRe = /^[A-Za-z_][A-Za-z0-9_]*$/;

function unquote(raw: string): string {
  const v = raw.trim();
  if (v.length >= 2 && v.startsWith('"') && v.endsWith('"')) {
    return v.slice(1, -1).replace(/\\n/g, "\n").replace(/\\"/g, '"').replace(/\\\\/g, "\\");
  }
  if (v.length >= 2 && v.startsWith("'") && v.endsWith("'")) return v.slice(1, -1);
  // An unquoted value ends at a comment.
  const hash = v.search(/\s#/);
  return hash >= 0 ? v.slice(0, hash).trimEnd() : v;
}

/** Parses KEY=value lines; blank lines, comments and "export " prefixes are allowed. */
export function parseDotenv(text: string): EnvParse {
  const vars: Record<string, string> = {};
  const errors: string[] = [];
  text.split(/\r?\n/).forEach((line, i) => {
    const t = line.trim();
    if (!t || t.startsWith("#")) return;
    const body = t.startsWith("export ") ? t.slice(7).trimStart() : t;
    const eq = body.indexOf("=");
    const key = eq > 0 ? body.slice(0, eq).trim() : "";
    if (!keyRe.test(key)) {
      errors.push(`Line ${i + 1}: expected KEY=value`);
      return;
    }
    vars[key] = unquote(body.slice(eq + 1));
  });
  return { vars, errors };
}

/** Quotes a value when .env needs it. */
export function formatDotenv(vars: Record<string, string>): string {
  return Object.keys(vars)
    .sort()
    .map((k) => {
      const v = vars[k] ?? "";
      return /^[A-Za-z0-9_./:@%+,-]*$/.test(v)
        ? `${k}=${v}`
        : `${k}="${v.replace(/\\/g, "\\\\").replace(/"/g, '\\"').replace(/\n/g, "\\n")}"`;
    })
    .join("\n");
}

export type EnvDiff = { added: string[]; changed: string[]; removed: string[] };

/**
 * Compares pasted variables with the editable ones. Keys that cannot be edited here
 * (masked or managed) are left out of "removed" when they are missing from the paste.
 */
export function diffDotenv(
  current: Record<string, string>,
  next: Record<string, string>,
  keep: Set<string>,
): EnvDiff {
  const added: string[] = [];
  const changed: string[] = [];
  const removed: string[] = [];
  for (const [k, v] of Object.entries(next)) {
    if (!(k in current) && !keep.has(k)) added.push(k);
    else if (k in current && current[k] !== v) changed.push(k);
    else if (keep.has(k) && !(k in current)) changed.push(k);
  }
  for (const k of Object.keys(current)) if (!(k in next) && !keep.has(k)) removed.push(k);
  return { added: added.sort(), changed: changed.sort(), removed: removed.sort() };
}
