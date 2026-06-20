/** Scope + pattern recovered from a persisted `custom_rule.rule_body`. */
export interface ParsedRuleBody {
  pattern: string;
  languages: string[];
  extensions: string[];
  includeGlobs: string[];
  excludeGlobs: string[];
}

/**
 * Parse a persisted `custom_rule.rule_body`. The server stores a JSON envelope
 * `{ pattern, languages, extensions, include_globs, exclude_globs }` (snake_case;
 * see `RuleBodyEnvelope` in crates/services/.../rule_authoring/types.rs). Legacy
 * or hand-seeded rows may be a bare pattern string; those fall back to the whole
 * value as the pattern with empty scope. Always returns a usable shape.
 */
export function parseRuleBody(ruleBody: string): ParsedRuleBody {
  const fallback: ParsedRuleBody = {
    pattern: ruleBody,
    languages: [],
    extensions: [],
    includeGlobs: [],
    excludeGlobs: [],
  };
  try {
    const parsed: unknown = JSON.parse(ruleBody);
    if (
      parsed &&
      typeof parsed === 'object' &&
      typeof (parsed as { pattern?: unknown }).pattern === 'string'
    ) {
      const o = parsed as Record<string, unknown>;
      const arr = (v: unknown): string[] =>
        Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
      return {
        pattern: o.pattern as string,
        languages: arr(o.languages),
        extensions: arr(o.extensions),
        includeGlobs: arr(o.include_globs ?? o.includeGlobs),
        excludeGlobs: arr(o.exclude_globs ?? o.excludeGlobs),
      };
    }
  } catch {
    // Not JSON -> a bare/legacy pattern row.
  }
  return fallback;
}
