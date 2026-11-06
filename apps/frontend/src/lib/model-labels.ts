// Friendly display names for coding-agent model ids (S10).
//
// The agent-sessions stats report raw ids such as "claude-opus-4-5-20251101" or
// "gpt-5.1-codex-max". This maps the known Claude and OpenAI/Codex id shapes to
// readable names; anything unrecognized is returned unchanged so new models
// still show up, just unprettified.

const FAMILIES: Record<string, string> = { opus: 'Opus', sonnet: 'Sonnet', haiku: 'Haiku' };

// claude-<family>-<major>[-<minor>][-<yyyymmdd>]   e.g. claude-sonnet-4-5-20250929
const CLAUDE_CURRENT = /^claude-(opus|sonnet|haiku)-(\d+)(?:-(\d{1,2}))?(?:-\d{8})?$/;
// claude-<major>[-<minor>]-<family>[-<yyyymmdd>]   e.g. claude-3-5-sonnet-20241022
const CLAUDE_LEGACY = /^claude-(\d+)(?:-(\d{1,2}))?-(opus|sonnet|haiku)(?:-\d{8})?$/;
// gpt-<version>[-<variant words>][-<yyyy-mm-dd>]   e.g. gpt-5.1-codex-max, gpt-4o-2024-08-06
const GPT = /^gpt-(\d+(?:\.\d+)?o?)((?:-[a-z]+)*)(?:-\d{4}-\d{2}-\d{2})?$/;

// OpenAI writes size tiers in lower case ("GPT-5 mini"); other words are capitalized.
const LOWERCASE_WORDS = new Set(['mini', 'nano']);

function version(major: string, minor: string | undefined): string {
  return minor ? `${major}.${minor}` : major;
}

function word(w: string): string {
  return LOWERCASE_WORDS.has(w) ? w : w.charAt(0).toUpperCase() + w.slice(1);
}

export function friendlyModelName(id: string): string {
  const key = id.trim().toLowerCase();

  let m = CLAUDE_CURRENT.exec(key);
  if (m) return `Claude ${FAMILIES[m[1] ?? ''] ?? ''} ${version(m[2] ?? '', m[3])}`;

  m = CLAUDE_LEGACY.exec(key);
  if (m) return `Claude ${version(m[1] ?? '', m[2])} ${FAMILIES[m[3] ?? ''] ?? ''}`;

  m = GPT.exec(key);
  if (m) {
    const words = (m[2] ?? '').split('-').filter(Boolean).map(word);
    return [`GPT-${m[1] ?? ''}`, ...words].join(' ');
  }

  return id;
}
