import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DiffView,
  DiffModeEnum,
  DiffLineType,
  parseInstance,
} from '@git-diff-view/react';
import { generateDiffFile, type DiffFile } from '@git-diff-view/file';
import { getHighLightLanguageFromPath } from '@/utils/extToLanguage';
import { useDiffViewMode } from '@/stores/useDiffViewStore';
import '@/styles/diff-style-overrides.css';

// Discriminated union for input format flexibility
export type DiffInput =
  | {
      type: 'content';
      oldContent: string;
      newContent: string;
      oldPath?: string;
      newPath: string;
    }
  | {
      type: 'unified';
      path: string;
      unifiedDiff: string;
      hasLineNumbers?: boolean;
    };

interface DiffData {
  diffFile: DiffFile | null;
  diffData: {
    hunks: string[];
    oldFile: { fileName: string; fileLang: string };
    newFile: { fileName: string; fileLang: string };
  } | null;
  additions: number;
  deletions: number;
  filePath: string;
  isValid: boolean;
  hideLineNumbers: boolean;
}

const EMPTY_DIFF: DiffData = {
  diffFile: null,
  diffData: null,
  additions: 0,
  deletions: 0,
  filePath: '',
  isValid: false,
  hideLineNumbers: false,
};

interface DiffStatCounts {
  additions: number;
  deletions: number;
}

function parseParsedDiffStats(unifiedDiff: string): DiffStatCounts {
  let additions = 0;
  let deletions = 0;
  const parsed = parseInstance.parse(unifiedDiff);
  for (const hunk of parsed.hunks) {
    for (const line of hunk.lines) {
      if (line.type === DiffLineType.Add) additions++;
      else if (line.type === DiffLineType.Delete) deletions++;
    }
  }
  return { additions, deletions };
}

function parseFallbackDiffStats(unifiedDiff: string): DiffStatCounts {
  let additions = 0;
  let deletions = 0;
  const lines = unifiedDiff.split('\n');
  for (const line of lines) {
    if (line.startsWith('+') && !line.startsWith('+++')) additions++;
    else if (line.startsWith('-') && !line.startsWith('---')) deletions++;
  }
  return { additions, deletions };
}

/**
 * Parse a unified diff to extract addition/deletion counts.
 * Uses the diff-view parser, falling back to a line-prefix count on parse failure.
 * Shared by ChatFileEntry stats (via NewDisplayConversationEntry) and processUnifiedDiff.
 */
export function parseDiffStats(unifiedDiff: string): DiffStatCounts {
  try {
    return parseParsedDiffStats(unifiedDiff);
  } catch {
    // Fallback: count lines starting with + or -
    return parseFallbackDiffStats(unifiedDiff);
  }
}

function processContentDiff(input: Extract<DiffInput, { type: 'content' }>): DiffData {
  const filePath = input.newPath || input.oldPath || 'unknown';
  const oldLang = getHighLightLanguageFromPath(input.oldPath || filePath) || 'plaintext';
  const newLang = getHighLightLanguageFromPath(input.newPath || filePath) || 'plaintext';
  const oldContent = input.oldContent || '';
  const newContent = input.newContent || '';

  if (oldContent === newContent) {
    return { ...EMPTY_DIFF, filePath };
  }

  try {
    const file = generateDiffFile(
      input.oldPath || filePath, oldContent,
      input.newPath || filePath, newContent,
      oldLang, newLang
    );
    file.initRaw();
    return {
      diffFile: file, diffData: null,
      additions: file.additionLength ?? 0,
      deletions: file.deletionLength ?? 0,
      filePath, isValid: true, hideLineNumbers: false,
    };
  } catch (e) {
    console.error('Failed to generate diff:', e);
    return { ...EMPTY_DIFF, filePath };
  }
}

function processUnifiedDiff(input: Extract<DiffInput, { type: 'unified' }>): DiffData {
  const { path, unifiedDiff, hasLineNumbers = true } = input;
  const lang = getHighLightLanguageFromPath(path) || 'plaintext';
  const { additions, deletions } = parseDiffStats(unifiedDiff);
  let isValid = false;

  try {
    isValid = parseInstance.parse(unifiedDiff).hunks.length > 0;
  } catch (e) {
    console.error('Failed to parse unified diff:', e);
  }

  return {
    diffFile: null,
    diffData: {
      hunks: [unifiedDiff],
      oldFile: { fileName: path, fileLang: lang },
      newFile: { fileName: path, fileLang: lang },
    },
    additions, deletions,
    filePath: path, isValid,
    hideLineNumbers: !hasLineNumbers,
  };
}

/**
 * Process input to get diff data and statistics
 */
function useDiffData(input: DiffInput): DiffData {
  return useMemo(() => {
    if (input.type === 'content') {
      return processContentDiff(input);
    }
    return processUnifiedDiff(input);
  }, [input]);
}

/**
 * Diff body component that renders the actual diff content
 * Can be used standalone (e.g., inside ChatFileEntry when expanded)
 */
export function DiffViewBody({
  diffFile,
  diffData,
  isValid,
  hideLineNumbers,
  theme,
}: Readonly<{
  diffFile: DiffFile | null;
  diffData: {
    hunks: string[];
    oldFile: { fileName: string; fileLang: string };
    newFile: { fileName: string; fileLang: string };
  } | null;
  isValid: boolean;
  hideLineNumbers?: boolean;
  theme: 'light' | 'dark';
}>) {
  const { t } = useTranslation('tasks');
  const globalMode = useDiffViewMode();
  const diffMode =
    globalMode === 'split' ? DiffModeEnum.Split : DiffModeEnum.Unified;

  if (!isValid) {
    return (
      <div className="px-base pb-base text-xs font-ibm-plex-mono text-low">
        {t('conversation.unableToRenderDiff')}
      </div>
    );
  }

  const wrapperClass = hideLineNumbers ? 'edit-diff-hide-nums' : '';

  // For content-based diff (Diff object)
  if (diffFile) {
    return (
      <div className={wrapperClass}>
        <DiffView
          diffFile={diffFile}
          diffViewWrap={false}
          diffViewTheme={theme}
          diffViewHighlight
          diffViewMode={diffMode}
          diffViewFontSize={12}
        />
      </div>
    );
  }

  // For unified diff string
  if (diffData) {
    return (
      <div className={wrapperClass}>
        <DiffView
          data={diffData}
          diffViewWrap={false}
          diffViewTheme={theme}
          diffViewHighlight
          diffViewMode={diffMode}
          diffViewFontSize={12}
        />
      </div>
    );
  }

  return null;
}

/**
 * Hook to process diff input - exported for use in ChatFileEntry
 */
export { useDiffData };
