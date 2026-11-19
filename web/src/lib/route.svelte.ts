// Hash routing, so the client can be served as static files from any path: #/<page>/…
export const pages = ['groups', 'nodes', 'settings'] as const;
export type Page = (typeof pages)[number];

const parse = (hash: string): Page => pages.find((p) => p === hash.split('/')[1]) ?? 'groups';

export const route = $state({ page: parse(location.hash) });

addEventListener('hashchange', () => {
  route.page = parse(location.hash);
});
