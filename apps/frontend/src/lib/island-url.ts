// Island names come from data-island attributes in server-rendered HTML; only
// the names Vite emits as /islands/<name>.js are accepted, so a crafted value
// can never turn the dynamic import into a path, scheme or query of its own.
const ISLAND_NAME = /^[a-z0-9-]+$/;

/** The bundle URL of the island `name`, or null when the name is not valid. */
export function islandModuleUrl(name: string): string | null {
  return ISLAND_NAME.test(name) ? `/islands/${name}.js` : null;
}
