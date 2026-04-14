import { useEffect, useCallback } from 'react';

type NavigatorWithUserAgentData = Navigator & {
  userAgentData?: {
    platform?: string;
  };
};

function getPlatformIdentifier(): string {
  const nav = navigator as NavigatorWithUserAgentData;
  return nav.userAgentData?.platform ?? navigator.userAgent;
}

/**
 * Hook that listens for CMD+K (Mac) or Ctrl+K (Windows/Linux) to open the command bar.
 * Uses native DOM event listener with capture phase to intercept before other handlers
 * like Lexical editor.
 */
export function useCommandBarShortcut(
  onOpen: () => void,
  enabled: boolean = true
) {
  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      // CMD+K (Mac) or Ctrl+K (Windows/Linux)
      const platform = getPlatformIdentifier();
      const isMac = platform.toUpperCase().includes('MAC');
      const modifier = isMac ? event.metaKey : event.ctrlKey;

      if (modifier && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        event.stopPropagation();
        onOpen();
      }
    },
    [onOpen]
  );

  useEffect(() => {
    if (typeof globalThis === 'undefined') return;
    if (!enabled) return;

    // Use capture phase to intercept before other handlers (like Lexical editor)
    globalThis.addEventListener('keydown', handleKeyDown, { capture: true });

    return () => {
      globalThis.removeEventListener('keydown', handleKeyDown, { capture: true });
    };
  }, [handleKeyDown, enabled]);
}
