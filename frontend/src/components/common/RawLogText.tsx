import { memo } from 'react';
import { AnsiHtml } from 'fancy-ansi/react';
import { hasAnsi } from 'fancy-ansi';
import { clsx } from 'clsx';

interface RawLogTextProps {
  readonly content: string;
  readonly channel?: 'stdout' | 'stderr';
  readonly as?: 'div' | 'span';
  readonly className?: string;
  readonly linkifyUrls?: boolean;
  readonly searchQuery?: string;
  readonly isCurrentMatch?: boolean;
}

const RawLogText = memo(
  ({
    content,
    channel = 'stdout',
    as: Component = 'div',
    className,
    linkifyUrls = false,
    searchQuery,
    isCurrentMatch = false,
  }: Readonly<RawLogTextProps>) => {
    // Only apply stderr fallback color when no ANSI codes are present
    const hasAnsiCodes = hasAnsi(content);
    const shouldApplyStderrFallback = channel === 'stderr' && !hasAnsiCodes;

    const highlightClass = isCurrentMatch
      ? 'bg-yellow-500/60 ring-1 ring-yellow-500 rounded-sm'
      : 'bg-yellow-500/30 rounded-sm';

    const highlightMatches = (text: string, key: string | number) => {
      if (!searchQuery) {
        return <AnsiHtml key={key} text={text} />;
      }

      const replacement = String.raw`\$&`;
      const escapedQuery = searchQuery.replaceAll(/[.*+?^${}()|[\]\\]/g, replacement);
      const regex = new RegExp(`(${escapedQuery})`, 'gi');
      const parts = text.split(regex);

      return parts.map((part, idx) => {
        const partKey = `${key}-${idx}-${part}`;
        if (part.toLowerCase() === searchQuery.toLowerCase()) {
          return (
            <mark key={partKey} className={highlightClass}>
              <AnsiHtml text={part} />
            </mark>
          );
        }
        return <AnsiHtml key={partKey} text={part} />;
      });
    };

    const renderContent = () => {
      if (!linkifyUrls) {
        return highlightMatches(content, 'content');
      }

      const urlRegex = /(https?:\/\/\S+)/g;
      const parts = content.split(urlRegex);

      return parts.map((part, idx) => {
        if (/^https?:\/\/\S+$/.test(part)) {
          return (
            <a
              key={`url-${idx}-${part}`}
              href={part}
              target="_blank"
              rel="noopener noreferrer"
              className="underline text-info hover:text-info/80 cursor-pointer"
              onClick={(e) => e.stopPropagation()}
            >
              {part}
            </a>
          );
        }
        // For non-URL parts, apply ANSI formatting with highlighting
        const partKey = `part-${idx}-${part.slice(0, 20)}`;
        return highlightMatches(part, partKey);
      });
    };

    return (
      <Component
        className={clsx(
          'font-mono text-xs break-all whitespace-pre-wrap',
          shouldApplyStderrFallback && 'text-error',
          className
        )}
      >
        {renderContent()}
      </Component>
    );
  }
);

RawLogText.displayName = 'RawLogText';

export default RawLogText;
