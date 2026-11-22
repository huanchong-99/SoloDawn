import { parse, routes, tabs, type Page, type Route } from './route';

export { href, parse, routes, tabs, type Page, type PageProps, type Tab } from './route';

/** Where a page stands along the navigation: the place of its entry; a page without one stands
 *  before the first entry, or after the last when only a development server opens it. */
function place(page: Page) {
  const route: Route = routes[page];
  if (route.tab) return tabs.indexOf(route.tab);
  return route.dev ? tabs.length : -1;
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
