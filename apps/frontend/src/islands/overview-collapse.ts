/**
 * Island: overview-collapse — runtime of the overview's collapsing hero (S3).
 *
 * Mounted on a hidden marker inside .overview. It measures the page, listens
 * to the scroller's scroll (passive, rAF-throttled) and writes the custom
 * properties computed by lib/overview-collapse.ts on .overview through the
 * CSSOM (element.style.setProperty — not an inline style attribute, so the
 * CSP style-src 'self' allows it). It also:
 *   - swaps which KPI version (full card or strip link) is exposed: the hidden
 *     one gets inert + aria-hidden, and focus inside it moves to its twin;
 *   - takes the faded hero controls ([data-collapse-fade]) out of the tab order
 *     once the hero is mostly collapsed;
 *   - pauses the hero graph (HERO_PAUSE_EVENT) while collapsed.
 * Below 1024px, or until measured, .overview has no data-collapse-ready and
 * the page scrolls as a plain layout. Listeners are removed when the overview
 * leaves the DOM (htmx swap) or the island is mounted again.
 */
import { collapseState, collapseVars, dockOffsets, type Rect } from '../lib/overview-collapse.ts';
import { heroPauseEvent } from '../lib/overview-hero.ts';

const KPI_RAMP = 120;
const WIDE = '(min-width: 1024px)';
const REDUCE = '(prefers-reduced-motion: reduce)';

let teardown: (() => void) | null = null;

function setHidden(el: Element | null, hidden: boolean): void {
  if (!(el instanceof HTMLElement)) return;
  el.inert = hidden;
  if (hidden) el.setAttribute('aria-hidden', 'true');
  else el.removeAttribute('aria-hidden');
}

export function mount(el: HTMLElement): void {
  teardown?.();
  const root = el.closest<HTMLElement>('.overview');
  const scroller = root?.closest<HTMLElement>('.main-content');
  const hero = root?.querySelector<HTMLElement>('#ov-hero');
  const heroUi = root?.querySelector<HTMLElement>('#ov-hero-ui');
  const kpis = root?.querySelector<HTMLElement>('#ov-kpis');
  const title = root?.querySelector<HTMLElement>('#ov-title');
  const form = root?.querySelector<HTMLElement>('#ov-search');
  if (!root || !scroller || !hero || !heroUi || !kpis || !title || !form) return;

  const wide = window.matchMedia(WIDE);
  const reduce = window.matchMedia(REDUCE);
  const cards = Array.from(kpis.querySelectorAll<HTMLElement>('.ov-kpi'));
  const fades = Array.from(root.querySelectorAll<HTMLElement>('[data-collapse-fade]'));
  let geometry = {
    heroTop: 0,
    heroH: 500,
    barH: 72,
    kpiOverlap: -64,
    ramp: KPI_RAMP,
    reduced: false,
  };
  let collapsed: boolean | null = null;
  let compact: boolean | null = null;
  let ticking = false;

  const set = (vars: Record<string, string>): void => {
    for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v);
  };

  function rel(node: HTMLElement, box: DOMRect): Rect {
    const r = node.getBoundingClientRect();
    return { x: r.left - box.left, y: r.top - box.top, h: r.height };
  }

  // Rest geometry, measured at progress 0 so transforms do not skew rects.
  function measure(): void {
    set({ '--ov-p': '0', '--ov-s': '0px', '--ov-k': '0' });
    const cs = getComputedStyle(root!);
    let heroTop = 0;
    for (let n = hero!.previousElementSibling; n; n = n.previousElementSibling) {
      heroTop += (n as HTMLElement).offsetHeight;
    }
    geometry = {
      heroTop,
      heroH: hero!.offsetHeight,
      barH: parseFloat(cs.getPropertyValue('--ov-bar-h')) || 72,
      kpiOverlap: parseFloat(getComputedStyle(kpis!).marginTop) || 0,
      ramp: KPI_RAMP,
      reduced: reduce.matches,
    };
    const box = heroUi!.getBoundingClientRect();
    const range = document.createRange();
    range.selectNodeContents(title!);
    set(
      dockOffsets({
        barH: geometry.barH,
        title: rel(title!, box),
        titleW: range.getBoundingClientRect().width,
        form: rel(form!, box),
        heroH: geometry.heroH,
      }),
    );
    let kh = 0;
    for (const c of cards)
      kh = Math.max(kh, c.querySelector<HTMLElement>('.ov-kpi-full')?.offsetHeight ?? 0);
    set({ '--ov-kpi-h': `${kh + 30}px` }); // 14px padding x2 + 1px border x2
  }

  function apply(): void {
    ticking = false;
    if (!root!.isConnected) {
      teardown?.();
      return;
    }
    const st = collapseState(scroller!.scrollTop, geometry);
    set(collapseVars(st));
    if (st.collapsed !== collapsed) {
      collapsed = st.collapsed;
      for (const f of fades) setHidden(f, st.collapsed);
      document.dispatchEvent(heroPauseEvent(st.collapsed));
    }
    if (st.compact !== compact) {
      compact = st.compact;
      for (const card of cards) {
        const full = card.querySelector('.ov-kpi-full');
        const mini = card.querySelector('.ov-kpi-mini');
        const from = st.compact ? full : mini;
        const to = st.compact ? mini : full;
        const refocus = from?.contains(document.activeElement) ?? false;
        setHidden(to, false);
        setHidden(from, true);
        if (refocus)
          (to?.matches('a') ? (to as HTMLElement) : to?.querySelector<HTMLElement>('a'))?.focus();
      }
    }
  }

  function reset(): void {
    root!.removeAttribute('data-collapse-ready');
    for (const k of [
      '--ov-p',
      '--ov-s',
      '--ov-k',
      '--ov-kpi-h',
      '--ov-h1-dy',
      '--ov-sx',
      '--ov-sy',
      '--ov-cv-dy',
    ]) {
      root!.style.removeProperty(k);
    }
    for (const f of fades) setHidden(f, false);
    for (const card of cards) {
      setHidden(card.querySelector('.ov-kpi-full'), false);
      setHidden(card.querySelector('.ov-kpi-mini'), true);
    }
    if (collapsed) document.dispatchEvent(heroPauseEvent(false));
    collapsed = null;
    compact = null;
  }

  function relayout(): void {
    if (!wide.matches) {
      reset();
      return;
    }
    measure();
    root!.setAttribute('data-collapse-ready', '');
    apply();
  }

  const onScroll = (): void => {
    if (!ticking && wide.matches) {
      ticking = true;
      requestAnimationFrame(apply);
    }
  };
  const onCleanup = (e: Event): void => {
    if (e.target instanceof Node && e.target.contains(root)) teardown?.();
  };
  const resize = new ResizeObserver(() => relayout());

  scroller.addEventListener('scroll', onScroll, { passive: true });
  wide.addEventListener('change', relayout);
  reduce.addEventListener('change', relayout);
  document.addEventListener('htmx:beforeCleanupElement', onCleanup);
  resize.observe(hero);
  // The agent cards fill in after load: re-measure when a full card resizes.
  for (const c of cards) {
    const full = c.querySelector('.ov-kpi-full');
    if (full) resize.observe(full);
  }

  teardown = () => {
    scroller.removeEventListener('scroll', onScroll);
    wide.removeEventListener('change', relayout);
    reduce.removeEventListener('change', relayout);
    document.removeEventListener('htmx:beforeCleanupElement', onCleanup);
    resize.disconnect();
    teardown = null;
  };
  relayout();
}
