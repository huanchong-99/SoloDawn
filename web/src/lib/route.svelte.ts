// Hash routing, so the client can be served as static files from any path: #/<path>. The table
// below holds every page: the path with its parameters, the page component in `routes/` that
// draws it, the entry of the navigation it belongs to, and whether it has the whole screen.

/** The entries of the navigation, in its order. */
export const tabs = ['groups', 'nodes', 'settings'] as const;
export type Tab = (typeof tabs)[number];

interface Route {
  /** Segments after `#/`: a word, a parameter `:name`, or an optional last one `:name?`. */
  path: string;
  /** The file in `routes/` without `.svelte`. Until it exists the page says it is coming. */
  view: string;
  /** The entry of the navigation that is current while the page is open. */
  tab?: Tab;
  /** The page has the screen to itself, without the top bar and the navigation: on a phone
   *  (below 768 px), or at every width. */
  full?: 'phone' | 'always';
  /** Only a development server opens it: the page reads the node's wire examples. */
  dev?: true;
}

/** The seven pages of the design (`web/design/index.html`), in its order. */
export const routes = {
  onboarding: { path: 'onboarding', view: 'Onboarding', full: 'always' },
  groups: { path: 'groups', view: 'Groups', tab: 'groups' },
  chat: { path: 'groups/:g', view: 'Groups', tab: 'groups', full: 'phone' },
  terminal: { path: 'terminal/:g/:member/:task?', view: 'Terminal', tab: 'groups', full: 'phone' },
  nodes: { path: 'nodes', view: 'Nodes', tab: 'nodes' },
  settings: { path: 'settings', view: 'Settings', tab: 'settings' },
  states: { path: 'states', view: 'dev/States', dev: true },
} as const satisfies Record<string, Route>;

export type Page = keyof typeof routes;
export const pages = Object.keys(routes) as Page[];

type Segments<S extends string> = S extends `${infer A}/${infer B}` ? A | Segments<B> : S;
type Names<P extends Page> =
  Segments<(typeof routes)[P]['path']> extends infer S
    ? S extends `:${infer N}`
      ? N
      : never
    : never;
/** The parameters of a page, from its path: `{ g: string }` for `groups/:g`. */
export type Params<P extends Page> = P extends Page
  ? { [N in Names<P> as N extends `${string}?` ? never : N]: string } & {
      [N in Names<P> as N extends `${infer M}?` ? M : never]?: string;
    }
  : never;

/** What every page component is given: the parameters of its address, and the connection. */
export interface PageProps<P extends Page = Page> {
  params: Params<P>;
  net: import('./net.svelte').Net;
}

/** Where a page stands along the navigation: the place of its entry; a page without one stands
 *  before the first entry, or after the last when only a development server opens it. */
function place(page: Page) {
  const route: Route = routes[page];
  if (route.tab) return tabs.indexOf(route.tab);
  return route.dev ? tabs.length : -1;
}

/** The parameters of a path, when the address fits it: as many segments, every word equal, and
 *  every parameter given but an optional last one. */
function fit(path: string, parts: string[]) {
  const segments = path.split('/');
  if (parts.length > segments.length) return undefined;
  const params: Record<string, string> = {};
  for (const [i, segment] of segments.entries()) {
    const part = parts[i] ?? '';
    if (!segment.startsWith(':')) {
      if (part !== segment) return undefined;
    } else if (part) {
      params[segment.slice(1).replace('?', '')] = part;
    } else if (!segment.endsWith('?')) {
      return undefined;
    }
  }
  return params;
}

/** The page an address opens, with its parameters. An address that fits no path, or the page of
 *  a development server in the build, opens the chats. No address fits two paths. */
export function parse(hash: string, dev = import.meta.env.DEV) {
  let parts: string[] = [];
  try {
    if (hash.startsWith('#/')) parts = hash.slice(2).split('/').map(decodeURIComponent);
  } catch {
    // A malformed escape: the address names no page.
  }
  for (const page of pages) {
    const route: Route = routes[page];
    const params = dev || !route.dev ? fit(route.path, parts) : undefined;
    if (params) return { page, params };
  }
  return { page: 'groups' as Page, params: {} };
}

/** The address of a page with its parameters. */
export function href<P extends Page>(
  page: P,
  ...[params]: {} extends Params<P> ? [Params<P>?] : [Params<P>]
) {
  const values: Record<string, string | undefined> = params ?? {};
  const parts = routes[page].path.split('/').map((segment) => {
    if (!segment.startsWith(':')) return segment;
    const value = values[segment.slice(1).replace('?', '')];
    return value === undefined ? undefined : encodeURIComponent(value);
  });
  return `#/${parts.filter((part) => part !== undefined).join('/')}`;
}

const first = parse(location.hash);

/** The page open now, its parameters, and the way the last change went along the navigation:
 *  1 to a later entry, -1 to an earlier one, 0 within one entry. */
export const route = $state({
  page: first.page,
  params: first.params as Record<string, string | undefined>,
  dir: 1,
});

addEventListener('hashchange', (event) => {
  const { page, params } = parse(new URL(event.newURL).hash);
  route.dir = Math.sign(place(page) - place(route.page));
  route.page = page;
  route.params = params;
});
