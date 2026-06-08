import type { ComponentType, JSX, ReactNode } from 'react';
import ReactMarkdown, { defaultUrlTransform, type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  ArrowRight,
  CheckCircle2,
  FileText,
  FolderTree,
  GraduationCap,
  Hash,
  HelpCircle,
  Lightbulb,
  Link2,
  ListChecks,
  Sparkles,
  Target,
  type LucideIcon,
} from 'lucide-react';

// engram `[[topic/key]]` wikilinks aren't standard markdown. We rewrite them to
// markdown links under a private `engram:` scheme so the regular markdown
// pipeline carries them, then intercept that scheme in the anchor renderer.
// The slug shape (no spaces/quotes) deliberately ignores bash `[[ … ]]` tests.
const WIKILINK_RE = /\[\[([A-Za-z0-9][\w/-]*)\]\]/g;
const WIKILINK_SCHEME = 'engram:';

function encodeWikilinks(markdown: string): string {
  return markdown.replace(WIKILINK_RE, (_full, target: string) => `[${target}](${WIKILINK_SCHEME}${target})`);
}

// Preserve our private scheme; defer everything else to react-markdown's default
// (which strips javascript: and other unsafe URLs).
function urlTransform(url: string): string {
  return url.startsWith(WIKILINK_SCHEME) ? url : defaultUrlTransform(url);
}

// Element styling for the observation markdown. We map each node to our HSL
// design tokens (plus Tailwind's default palette for the section accents)
// instead of pulling in @tailwindcss/typography — that keeps the rendered memory
// visually consistent with the rest of the panel and avoids the `prose` reset
// fighting our own scale. Font size inherits from the wrapper (13px).
const COMPONENTS: Components = {
  h1: ({ children }) => (
    <h1 className="mt-3 mb-1 text-sm font-semibold text-fg first:mt-0">{children}</h1>
  ),
  h2: ({ children }) => (
    <h2 className="mt-3 mb-1 text-xs font-semibold uppercase tracking-wide text-fg-muted first:mt-0">
      {children}
    </h2>
  ),
  h3: ({ children }) => (
    <h3 className="mt-2 mb-1 font-semibold text-fg first:mt-0">{children}</h3>
  ),
  p: ({ children }) => <p className="my-2 leading-relaxed text-fg first:mt-0 last:mb-0">{children}</p>,
  strong: ({ children }) => <strong className="font-semibold text-fg">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
  ul: ({ children }) => <ul className="my-2 list-disc space-y-1 pl-5 marker:text-fg-muted">{children}</ul>,
  ol: ({ children }) => <ol className="my-2 list-decimal space-y-1 pl-5 marker:text-fg-muted">{children}</ol>,
  li: ({ children }) => <li className="leading-relaxed text-fg">{children}</li>,
  code: ({ children }) => (
    <code className="rounded bg-surface-2 px-1 py-0.5 font-mono text-[11px] text-fg">
      {children}
    </code>
  ),
  pre: ({ children }) => (
    <pre className="my-2 overflow-x-auto rounded-md bg-surface-2 p-2 font-mono text-[11px] leading-relaxed text-fg">
      {children}
    </pre>
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-2 border-l-2 border-border pl-3 text-fg-muted">{children}</blockquote>
  ),
  hr: () => <hr className="my-3 border-border" />,
  table: ({ children }) => (
    <div className="my-2 overflow-x-auto">
      <table className="w-full border-collapse text-[11px]">{children}</table>
    </div>
  ),
  th: ({ children }) => (
    <th className="border border-border px-2 py-1 text-left font-semibold text-fg">{children}</th>
  ),
  td: ({ children }) => <td className="border border-border px-2 py-1 text-fg">{children}</td>,
};

// Visual identity per known engram label. `bar` tints the left rule, `text`
// tints the label + icon. Unknown labels fall back to a neutral style.
interface LabelStyle {
  text: string;
  bar: string;
  icon: LucideIcon;
}
const LABEL_STYLES: Record<string, LabelStyle> = {
  what: { text: 'text-sky-300', bar: 'border-sky-400/60', icon: Sparkles },
  why: { text: 'text-violet-300', bar: 'border-violet-400/60', icon: HelpCircle },
  where: { text: 'text-emerald-300', bar: 'border-emerald-400/60', icon: FolderTree },
  learned: { text: 'text-amber-300', bar: 'border-amber-400/60', icon: GraduationCap },
  goal: { text: 'text-sky-300', bar: 'border-sky-400/60', icon: Target },
  instructions: { text: 'text-cyan-300', bar: 'border-cyan-400/60', icon: ListChecks },
  discoveries: { text: 'text-amber-300', bar: 'border-amber-400/60', icon: Lightbulb },
  accomplished: { text: 'text-emerald-300', bar: 'border-emerald-400/60', icon: CheckCircle2 },
  'next steps': { text: 'text-blue-300', bar: 'border-blue-400/60', icon: ArrowRight },
  'relevant files': { text: 'text-slate-300', bar: 'border-slate-400/60', icon: FileText },
};
const DEFAULT_STYLE: LabelStyle = { text: 'text-fg-muted', bar: 'border-border', icon: Hash };

interface Section {
  label: string | null;
  body: string;
}

// Split the content into `**Label**:`-prefixed sections. Anything before the
// first label (e.g. a `# Heading` design doc) becomes a label-less preamble that
// renders as plain markdown — so non-conforming content degrades gracefully.
function parseSections(content: string): Section[] {
  const re = /(?:^|\n\n?)\*\*([A-Za-z][A-Za-z /&]*)\*\*:[ \t]*/g;
  const hits: Array<{ label: string; start: number; bodyStart: number }> = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(content)) !== null) {
    hits.push({ label: (m[1] ?? '').trim(), start: m.index, bodyStart: re.lastIndex });
  }
  const first = hits[0];
  if (first === undefined) return [{ label: null, body: content.trim() }];

  const sections: Section[] = [];
  const preamble = content.slice(0, first.start).trim();
  if (preamble) sections.push({ label: null, body: preamble });
  for (const [i, cur] of hits.entries()) {
    const next = hits[i + 1];
    const end = next !== undefined ? next.start : content.length;
    sections.push({ label: cur.label, body: content.slice(cur.bodyStart, end).trim() });
  }
  return sections;
}

// A `[[topic/key]]` reference. Clickable when the host wired `onWikilink`
// (the full /brain page), otherwise a static pill (e.g. compact preview cards).
function WikiChip({
  target,
  label,
  onWikilink,
}: {
  target: string;
  label: ReactNode;
  onWikilink?: ((target: string) => void) | undefined;
}): JSX.Element {
  const base =
    'inline-flex items-center gap-1 rounded bg-accent/10 px-1.5 py-px align-baseline font-mono text-[11px] text-accent';
  if (onWikilink === undefined) {
    return (
      <span className={base}>
        <Link2 className="size-3 shrink-0" aria-hidden />
        {label}
      </span>
    );
  }
  return (
    <button
      type="button"
      onClick={() => onWikilink(target)}
      className={`${base} transition-colors hover:bg-accent/20`}
    >
      <Link2 className="size-3 shrink-0" aria-hidden />
      {label}
    </button>
  );
}

function buildComponents(onWikilink?: ((target: string) => void) | undefined): Components {
  return {
    ...COMPONENTS,
    a: ({ href, children }) => {
      if (href !== undefined && href.startsWith(WIKILINK_SCHEME)) {
        return (
          <WikiChip target={href.slice(WIKILINK_SCHEME.length)} label={children} onWikilink={onWikilink} />
        );
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noreferrer"
          className="text-accent underline decoration-accent/40 underline-offset-2 hover:decoration-accent"
        >
          {children}
        </a>
      );
    },
  };
}

function MarkdownBlock({
  content,
  onWikilink,
}: {
  content: string;
  onWikilink?: ((target: string) => void) | undefined;
}): JSX.Element {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      urlTransform={urlTransform}
      components={buildComponents(onWikilink)}
    >
      {encodeWikilinks(content)}
    </ReactMarkdown>
  );
}

function SectionCard({
  label,
  body,
  onWikilink,
}: Section & { onWikilink?: ((target: string) => void) | undefined }): JSX.Element {
  if (label === null) return <MarkdownBlock content={body} onWikilink={onWikilink} />;
  const style = LABEL_STYLES[label.toLowerCase()] ?? DEFAULT_STYLE;
  const Icon: ComponentType<{ className?: string }> = style.icon;
  return (
    <section className={`border-l-2 pl-3 ${style.bar}`}>
      <div
        className={`mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide ${style.text}`}
      >
        <Icon className="size-3.5 shrink-0" />
        <span>{label}</span>
      </div>
      <MarkdownBlock content={body} onWikilink={onWikilink} />
    </section>
  );
}

export interface MarkdownContentProps {
  content: string;
  /**
   * Invoked when a `[[topic/key]]` wikilink chip is clicked, with the bare
   * topic key. When omitted, wikilinks render as static (non-clickable) pills.
   */
  onWikilink?: ((target: string) => void) | undefined;
}

// Renders observation content as GFM markdown, grouped into colored sections by
// the engram `**Label**:` convention. react-markdown builds a React tree from
// the AST (no dangerouslySetInnerHTML), so untrusted DB content is XSS-safe by
// construction.
export default function MarkdownContent({ content, onWikilink }: MarkdownContentProps): JSX.Element {
  const sections = parseSections(content);
  return (
    <div className="space-y-3 break-words text-[13px] text-fg">
      {sections.map((s, i) => (
        <SectionCard
          key={`${s.label ?? 'preamble'}-${i}`}
          label={s.label}
          body={s.body}
          onWikilink={onWikilink}
        />
      ))}
    </div>
  );
}
