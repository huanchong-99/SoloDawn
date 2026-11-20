// Hash routing, so the client can be served as static files from any path: #/<page>/…
export const pages = ['groups', 'nodes', 'settings'] as const;
export type Page = (typeof pages)[number];

const parse = (hash: string): Page => pages.find((p) => p === hash.split('/')[1]) ?? 'groups';

// dir is the way the last change went along the navigation: 1 forward, -1 back.
export const route = $state({ page: parse(location.hash), dir: 1 });

addEventListener('hashchange', (event) => {
  const page = parse(new URL(event.newURL).hash);
  route.dir = Math.sign(pages.indexOf(page) - pages.indexOf(route.page));
  route.page = page;
});
