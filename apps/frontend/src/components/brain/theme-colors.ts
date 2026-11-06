// Theme-aware colors for the WebGL brain (S9).
//
// The design tokens (internal/ui/static/tokens.css) are bare HSL triplets such
// as `--info: 267 57% 78%`, consumed in CSS as hsl(var(--info)). Three.js cannot
// read CSS variables, so the scene resolves them once via getComputedStyle and
// converts the triplet into a comma hsl() string that THREE.Color parses. The
// fallbacks are the previous hard-coded Rosé Pine values, used when a token is
// missing or outside a browser (tests, SSR).

const HSL_TRIPLET = /^(\d*\.?\d+)\s+(\d*\.?\d+)%\s+(\d*\.?\d+)%$/;
const FULL_COLOR = /^(#[0-9a-f]{3,8}|(rgb|rgba|hsl|hsla)\([^)]*\))$/i;

/** Convert a token value into a CSS color string THREE.Color understands. */
export function cssColorFromToken(raw: string, fallback: string): string {
  const v = raw.trim();
  const m = HSL_TRIPLET.exec(v);
  if (m) return `hsl(${m[1] ?? '0'}, ${m[2] ?? '0'}%, ${m[3] ?? '0'}%)`;
  if (FULL_COLOR.test(v)) return v;
  return fallback;
}

type TokenReader = (name: string) => string;

function documentReader(): TokenReader | null {
  if (typeof document === 'undefined' || typeof getComputedStyle === 'undefined') return null;
  const style = getComputedStyle(document.documentElement);
  return (name) => style.getPropertyValue(name);
}

/** Resolve a design token (e.g. '--info') to a CSS color, with a fallback. */
export function readThemeColor(name: string, fallback: string, read?: TokenReader): string {
  const reader = read ?? documentReader();
  if (!reader) return fallback;
  return cssColorFromToken(reader(name), fallback);
}

// Token names used by the brain scene, with their pre-token fallbacks.
/** Iris rim light on the nodes (was #c4a7e7). */
export const RIM_LIGHT_TOKEN = '--info';
export const RIM_LIGHT_FALLBACK = '#c4a7e7';
/** Nodes / legend entries with no value under the active colorBy (was #6e6a86). */
export const NULL_COLOR_TOKEN = '--fg-muted';
export const NULL_COLOR_FALLBACK = '#6e6a86';
