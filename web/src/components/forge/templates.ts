import type { GroupRow } from '../../lib/proto';

export type Templates = NonNullable<GroupRow['templates']>;
export const operations = ['push', 'open', 'checks', 'merge'] as const;

export function emptyTemplates(): Templates {
  return { push: [''], open: [''], checks: [''], merge: [''] };
}
