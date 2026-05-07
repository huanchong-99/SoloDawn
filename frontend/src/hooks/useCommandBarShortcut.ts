import { useEffect, useCallback } from 'react';

type NavigatorWithUserAgentData = Navigator & {
  userAgentData?: {
    platform?: string;
  };
};

function getPlatformIdentifier(): string {
  if (typeof navigator === 'undefined') return '';
  const nav = navigator as NavigatorWithUserAgentData;
  return nav.userAgentData?.platform ?? navigator.userAgent;
}

// Hoist platform detection to module scope: it cannot change within a
// session, so repeating the lookup per-render (and per-keystroke) wastes
// work. [E17-10]
const PLATFORM_IDENTIFIER = getPlatformIdentifier();
const IS_MAC_PLATFORM = PLATFORM_IDENTIFIER.toUpperCase().includes('MAC');

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
      const modifier = IS_MAC_PLATFORM ? event.metaKey : event.ctrlKey;

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
