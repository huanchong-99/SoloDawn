import { useCallback, useMemo, useRef, useEffect } from 'react';
import { useScratch } from './useScratch';
import { useDebouncedCallback } from './useDebouncedCallback';
import {
  ScratchType,
  type PreviewSettingsData,
} from 'shared/types';

export type ScreenSize = 'desktop' | 'mobile' | 'responsive';

export interface ResponsiveDimensions {
  width: number;
  height: number;
}

interface UsePreviewSettingsResult {
  // URL override
  overrideUrl: string | null;
  hasOverride: boolean;
  setOverrideUrl: (url: string) => void;
  clearOverride: () => Promise<void>;

  // Screen size
  screenSize: ScreenSize;
  responsiveDimensions: ResponsiveDimensions;
  setScreenSize: (size: ScreenSize) => void;
  setResponsiveDimensions: (dimensions: ResponsiveDimensions) => void;

  isLoading: boolean;
}

const DEFAULT_RESPONSIVE_DIMENSIONS: ResponsiveDimensions = {
  width: 800,
  height: 600,
};

/**
 * Hook to manage per-workspace preview settings (URL override and screen size).
 * Uses the scratch system for persistence.
 */
export function usePreviewSettings(
  workspaceId: string | undefined
): UsePreviewSettingsResult {
  const enabled = !!workspaceId;

  const {
    scratch,
    updateScratch,
    deleteScratch,
    isLoading: isScratchLoading,
  } = useScratch(ScratchType.PREVIEW_SETTINGS, workspaceId ?? '', {
    enabled,
  });

  // Extract settings from scratch data
  const payload = scratch?.payload;
  const scratchData: PreviewSettingsData | undefined =
    payload?.type === 'PREVIEW_SETTINGS' ? payload.data : undefined;

  const overrideUrl = scratchData?.url ?? null;
  const hasOverride = overrideUrl !== null && overrideUrl.trim() !== '';

  const screenSize: ScreenSize =
    (scratchData?.screenSize as ScreenSize) ?? 'desktop';
  const responsiveDimensions: ResponsiveDimensions = useMemo(
    () => ({
      width:
        scratchData?.responsiveWidth ?? DEFAULT_RESPONSIVE_DIMENSIONS.width,
      height:
        scratchData?.responsiveHeight ?? DEFAULT_RESPONSIVE_DIMENSIONS.height,
    }),
    [scratchData?.responsiveWidth, scratchData?.responsiveHeight]
  );

  // Keep latest values in refs so saveSettings has a stable identity.
  // This breaks the circular dep where saveSettings → responsiveDimensions →
  // setResponsiveDimensions → saveSettings.
  const latestRef = useRef({
    overrideUrl,
    screenSize,
    responsiveWidth: responsiveDimensions.width,
    responsiveHeight: responsiveDimensions.height,
  });
  useEffect(() => {
    latestRef.current = {
      overrideUrl,
      screenSize,
      responsiveWidth: responsiveDimensions.width,
      responsiveHeight: responsiveDimensions.height,
    };
  }, [
    overrideUrl,
    screenSize,
    responsiveDimensions.width,
    responsiveDimensions.height,
  ]);

  // Helper to save settings
  const saveSettings = useCallback(
    async (updates: Partial<PreviewSettingsData>) => {
      if (!workspaceId) return;

      const latest = latestRef.current;
      try {
        await updateScratch({
          payload: {
            type: 'PREVIEW_SETTINGS',
            data: {
              url: updates.url ?? latest.overrideUrl ?? '',
              screenSize: updates.screenSize ?? latest.screenSize,
              responsiveWidth:
                updates.responsiveWidth ?? latest.responsiveWidth,
              responsiveHeight:
                updates.responsiveHeight ?? latest.responsiveHeight,
            },
          },
        });
      } catch (e) {
        console.error('[usePreviewSettings] Failed to save:', e);
      }
    },
    [workspaceId, updateScratch]
  );

  // Debounced save for URL changes (frequent typing)
  const { debounced: debouncedSaveUrl } = useDebouncedCallback(
    async (url: string) => {
      await saveSettings({ url });
    },
    300
  );

  // Debounced save for responsive dimensions (frequent dragging)
  const { debounced: debouncedSaveDimensions } = useDebouncedCallback(
    async (dimensions: ResponsiveDimensions) => {
      await saveSettings({
        responsiveWidth: dimensions.width,
        responsiveHeight: dimensions.height,
      });
    },
    300
  );

  const setOverrideUrl = useCallback(
    (url: string) => {
      debouncedSaveUrl(url);
    },
    [debouncedSaveUrl]
  );

  const setScreenSize = useCallback(
    (size: ScreenSize) => {
      saveSettings({ screenSize: size });
    },
    [saveSettings]
  );

  const setResponsiveDimensions = useCallback(
    (dimensions: ResponsiveDimensions) => {
      debouncedSaveDimensions(dimensions);
    },
    [debouncedSaveDimensions]
  );

  const clearOverride = useCallback(async () => {
    try {
      await deleteScratch();
    } catch (e) {
      // Ignore 404 errors when scratch doesn't exist
      console.error('[usePreviewSettings] Failed to clear:', e);
    }
  }, [deleteScratch]);

  return {
    overrideUrl,
    hasOverride,
    setOverrideUrl,
    clearOverride,
    screenSize,
    responsiveDimensions,
    setScreenSize,
    setResponsiveDimensions,
    isLoading: isScratchLoading,
  };
}
