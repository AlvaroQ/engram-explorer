/**
 * Islands loader — mounts React islands declared in the HTML.
 *
 * Usage (emitted by the templ Island() helper):
 *   <div data-island="type-breakdown" data-props='{"by_type":[...]}'></div>
 *   <script type="module" src="/islands/loader.js"></script>
 *
 * How it works:
 *   1. Scans all [data-island] divs in the document.
 *   2. Parses data-props as JSON (defaults to {} if absent/invalid).
 *   3. Dynamic-imports /islands/<name>.js.
 *   4. Calls the exported mount(el, props) function.
 *
 * Adding a new island:
 *   - Create src/islands/<name>.tsx exporting mount(el, props).
 *   - Add it to vite.config.ts build.rollupOptions.input (see comment there).
 *   - Use @island("<name>", data) in your templ page.
 *   - No changes needed to this loader.
 */

type IslandModule = {
  mount: (el: HTMLElement, props: unknown) => void;
};

async function mountIsland(el: HTMLElement): Promise<void> {
  const name = el.dataset['island'];
  if (!name) return;

  // Parse props — data-props is optional; default to empty object.
  let props: unknown = {};
  const raw = el.dataset['props'];
  if (raw) {
    try {
      props = JSON.parse(raw);
    } catch (err) {
      console.error(`[islands] Failed to parse data-props for island "${name}":`, err);
    }
  }

  // Dynamic import of the island bundle. Vite emits stable paths so the URL
  // is predictable at runtime without a manifest lookup.
  const url = `/islands/${name}.js`;
  let mod: IslandModule;
  try {
    mod = (await import(/* @vite-ignore */ url)) as IslandModule;
  } catch (err) {
    console.error(`[islands] Failed to load bundle for island "${name}" from ${url}:`, err);
    return;
  }

  if (typeof mod.mount !== 'function') {
    console.error(`[islands] Island "${name}" does not export a mount() function.`);
    return;
  }

  try {
    mod.mount(el, props);
  } catch (err) {
    console.error(`[islands] Error mounting island "${name}":`, err);
  }
}

// Mount all islands present at DOMContentLoaded. Islands added dynamically
// (e.g. via HTMX swap) should call mountIsland(el) directly after insertion
// or listen for htmx:afterSwap and re-scan.
function mountAll(): void {
  const nodes = document.querySelectorAll<HTMLElement>('[data-island]');
  nodes.forEach((el) => {
    void mountIsland(el);
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  // Already parsed (e.g. script placed at end of body or deferred in a module).
  mountAll();
}

// Exported for programmatic use (e.g. after HTMX swaps).
export { mountIsland, mountAll };
