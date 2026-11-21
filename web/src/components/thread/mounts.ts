// Cards and rows owned by the thread area: scored reviews, check results, system notices
// (including task_merged), completion report metadata and takeover hand-back metadata.
import type { Mounts } from '../../lib/mounts.svelte';
import ChecksRow from './ChecksRow.svelte';
import HandbackMeta from './HandbackMeta.svelte';
import ReportMeta from './ReportMeta.svelte';
import ReviewBlock from './ReviewBlock.svelte';
import SystemRow from './SystemRow.svelte';

export default {
  cards: {
    review: ReviewBlock,
    checks: ChecksRow,
    system: SystemRow,
    report: ReportMeta,
    handback: HandbackMeta,
  },
} satisfies Mounts;
