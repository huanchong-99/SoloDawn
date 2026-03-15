import type { RefObject } from 'react';
import {
  PlayIcon,
  SpinnerIcon,
  WrenchIcon,
  ArrowSquareOutIcon,
  ArrowClockwiseIcon,
  CopyIcon,
  XIcon,
  MonitorIcon,
  DeviceMobileIcon,
  ArrowsOutCardinalIcon,
  PauseIcon,
} from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { cn } from '@/lib/utils';
import { PrimaryButton } from '../primitives/PrimaryButton';
import {
  IconButtonGroup,
  IconButtonGroupItem,
} from '../primitives/IconButtonGroup';
import type { Repo } from 'shared/types';
import type {
  ScreenSize,
  ResponsiveDimensions,
} from '@/hooks/usePreviewSettings';

export const MOBILE_WIDTH = 390;
export const MOBILE_HEIGHT = 844;
// Phone frame adds padding (p-3 = 12px * 2) and rounded corners
export const PHONE_FRAME_PADDING = 24;

interface PreviewBrowserProps {
  url?: string;
  autoDetectedUrl?: string;
  urlInputValue: string;
  urlInputRef: RefObject<HTMLInputElement>;
  isUsingOverride?: boolean;
  onUrlInputChange: (value: string) => void;
  onClearOverride?: () => void;
  onCopyUrl: () => void;
  onOpenInNewTab: () => void;
  onRefresh: () => void;
  onStart: () => void;
  onStop: () => void;
  isStarting: boolean;
  isStopping: boolean;
  isServerRunning: boolean;
  screenSize: ScreenSize;
  localDimensions: ResponsiveDimensions;
  onScreenSizeChange: (size: ScreenSize) => void;
  onResizeStart: (
    direction: 'right' | 'bottom' | 'corner'
  ) => (e: React.MouseEvent | React.TouchEvent) => void;
  isResizing: boolean;
  containerRef: RefObject<HTMLDivElement>;
  repos: Repo[];
  handleEditDevScript: () => void;
  handleFixDevScript?: () => void;
  hasFailedDevServer?: boolean;
  mobileScale: number;
  className?: string;
}

// Helper to get start/stop button icon
function getServerControlIcon(
  isServerRunning: boolean,
  isStarting: boolean,
  isStopping: boolean
) {
  if (isServerRunning) {
    return isStopping ? SpinnerIcon : PauseIcon;
  }
  return isStarting ? SpinnerIcon : PlayIcon;
}

// Helper to check if icon should animate
function shouldAnimateIcon(
  isServerRunning: boolean,
  isStarting: boolean,
  isStopping: boolean
): boolean {
  return (isServerRunning && isStopping) || (!isServerRunning && isStarting);
}

// Helper to get disabled state for start/stop button
function getServerControlDisabled(
  isServerRunning: boolean,
  isStarting: boolean,
  isStopping: boolean,
  hasDevScript: boolean
): boolean {
  if (isServerRunning) {
    return isStopping;
  }
  return isStarting || !hasDevScript;
}

// Helper component for empty state content
function EmptyStateContent({
  isLoading,
  isStarting,
  hasDevScript,
  hasFailedDevServer,
  onStart,
  handleEditDevScript,
  handleFixDevScript,
  t,
}: Readonly<{
  isLoading: boolean;
  isStarting: boolean;
  hasDevScript: boolean;
  hasFailedDevServer?: boolean;
  onStart: () => void;
  handleEditDevScript: () => void;
  handleFixDevScript?: () => void;
  t: TFunction;
}>) {
  if (isLoading) {
    return (
      <>
        <SpinnerIcon className="size-icon-lg animate-spin text-brand" />
        <p className="text-sm">
          {isStarting
            ? t('preview.loading.startingServer')
            : t('preview.loading.waitingForServer')}
        </p>
      </>
    );
  }

  if (hasDevScript) {
    return (
      <>
        <p>{t('preview.noServer.title')}</p>
        {hasFailedDevServer && handleFixDevScript ? (
          <PrimaryButton
            variant="tertiary"
            value={t('scriptFixer.fixScript')}
            actionIcon={WrenchIcon}
            onClick={handleFixDevScript}
          />
        ) : (
          <PrimaryButton
            value={t('attempt.actions.startDevServer')}
            actionIcon={PlayIcon}
            onClick={onStart}
          />
        )}
      </>
    );
  }

  return (
    <div className="flex flex-col gap-double p-double max-w-md">
      <div className="flex flex-col gap-base">
        <p className="text-xl text-high max-w-xs">
          {t('preview.noServer.setupTitle')}
        </p>
        <p>{t('preview.noServer.setupPrompt')}</p>
      </div>
      <div className="flex flex-col gap-base">
        <div>
          <PrimaryButton
            value={t('preview.noServer.editDevScript')}
            onClick={handleEditDevScript}
          />
        </div>
        <a
          href="https://www.gitcortex.com/docs/core-features/testing-your-application"
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand hover:text-brand-hover underline"
        >
          {t('preview.noServer.learnMore')}
        </a>
      </div>
    </div>
  );
}

// Helper component for mobile iframe view
function MobileIframeView({
  url,
  title,
  mobileScale,
}: Readonly<{
  url: string;
  title: string;
  mobileScale: number;
}>) {
  return (
    <div
      className="bg-primary rounded-[2rem] p-3 shadow-xl origin-center"
      style={{ transform: mobileScale < 1 ? `scale(${mobileScale})` : undefined }}
    >
      <div
        className="rounded-[1.5rem] overflow-hidden"
        style={{ width: MOBILE_WIDTH, height: MOBILE_HEIGHT }}
      >
        <iframe
          src={url}
          title={title}
          className="w-full h-full border-0"
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
          referrerPolicy="no-referrer"
        />
      </div>
    </div>
  );
}

// Helper component for desktop/responsive iframe view
function DesktopIframeView({
  url,
  title,
  screenSize,
  isResizing,
  containerStyle,
  onResizeStart,
  t,
}: Readonly<{
  url: string;
  title: string;
  screenSize: ScreenSize;
  isResizing: boolean;
  containerStyle: React.CSSProperties;
  onResizeStart: (direction: 'right' | 'bottom' | 'corner') => (e: React.MouseEvent | React.TouchEvent) => void;
  t: (key: string) => string;
}>) {
  return (
    <div
      className={cn(
        'rounded-sm border overflow-hidden relative',
        screenSize === 'responsive' && 'shadow-lg'
      )}
      style={containerStyle}
    >
      <iframe
        src={url}
        title={title}
        className={cn('w-full h-full border-0', isResizing && 'pointer-events-none')}
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        referrerPolicy="no-referrer"
      />
      {screenSize === 'responsive' && (
        <>
          <button
            type="button"
            className="absolute top-0 right-0 w-2 h-full cursor-ew-resize hover:bg-brand/30 transition-colors bg-transparent border-0 p-0 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-brand"
            onMouseDown={onResizeStart('right')}
            onTouchStart={onResizeStart('right')}
            aria-label={t('preview.toolbar.resizeWidth')}
          />
          <button
            type="button"
            className="absolute bottom-0 left-0 w-full h-2 cursor-ns-resize hover:bg-brand/30 transition-colors bg-transparent border-0 p-0 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-brand"
            onMouseDown={onResizeStart('bottom')}
            onTouchStart={onResizeStart('bottom')}
            aria-label={t('preview.toolbar.resizeHeight')}
          />
          <button
            type="button"
            className="absolute bottom-0 right-0 w-4 h-4 cursor-nwse-resize hover:bg-brand/30 transition-colors bg-transparent border-0 p-0 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-brand"
            onMouseDown={onResizeStart('corner')}
            onTouchStart={onResizeStart('corner')}
            aria-label={t('preview.toolbar.resizeBoth')}
          />
        </>
      )}
    </div>
  );
}

export function PreviewBrowser({
  url,
  autoDetectedUrl,
  urlInputValue,
  urlInputRef,
  isUsingOverride,
  onUrlInputChange,
  onClearOverride,
  onCopyUrl,
  onOpenInNewTab,
  onRefresh,
  onStart,
  onStop,
  isStarting,
  isStopping,
  isServerRunning,
  screenSize,
  localDimensions,
  onScreenSizeChange,
  onResizeStart,
  isResizing,
  containerRef,
  repos,
  handleEditDevScript,
  handleFixDevScript,
  hasFailedDevServer,
  mobileScale,
  className,
}: Readonly<PreviewBrowserProps>) {
  const { t } = useTranslation(['tasks', 'common']);
  const isLoading = isStarting || (isServerRunning && !url);
  const showIframe = url && !isLoading && isServerRunning;

  const hasDevScript = repos.some(
    (repo) => !!repo.devServerScript?.trim()
  );

  const getIframeContainerStyle = (): React.CSSProperties => {
    switch (screenSize) {
      case 'mobile':
        return {
          width: MOBILE_WIDTH,
          height: MOBILE_HEIGHT,
        };
      case 'responsive':
        return {
          width: localDimensions.width,
          height: localDimensions.height,
        };
      case 'desktop':
      default:
        return {
          width: '100%',
          height: '100%',
        };
    }
  };

  return (
    <div
      className={cn(
        'bg-brand/20 w-full h-full flex flex-col overflow-hidden',
        className
      )}
    >
      {/* Floating Toolbar */}
      <div className="p-double">
        <div className="backdrop-blur-sm bg-primary/80 border border-brand/20 flex items-center gap-base p-base rounded-md shadow-md shrink-0">
          {/* URL Input */}
          <div
            className={cn(
              'flex items-center gap-half rounded-sm px-base py-half flex-1 min-w-0',
              !isServerRunning && 'opacity-50'
            )}
          >
            <input
              ref={urlInputRef}
              type="text"
              value={urlInputValue}
              onChange={(e) => onUrlInputChange(e.target.value)}
              placeholder={autoDetectedUrl ?? 'Enter URL...'}
              disabled={!isServerRunning}
              className={cn(
                'flex-1 font-mono text-sm bg-transparent border-none outline-none min-w-0',
                isUsingOverride
                  ? 'text-normal'
                  : 'text-low placeholder:text-low',
                !isServerRunning && 'cursor-not-allowed'
              )}
            />
          </div>

          {/* URL Actions */}
          <IconButtonGroup>
            {isUsingOverride && (
              <IconButtonGroupItem
                icon={XIcon}
                onClick={onClearOverride}
                disabled={!isServerRunning}
                aria-label={t('preview.toolbar.clearUrlOverride')}
                title={t('preview.toolbar.resetUrl')}
              />
            )}
            <IconButtonGroupItem
              icon={CopyIcon}
              onClick={onCopyUrl}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.copyUrl')}
              title={t('preview.toolbar.copyUrl')}
            />
            <IconButtonGroupItem
              icon={ArrowSquareOutIcon}
              onClick={onOpenInNewTab}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.openInTab')}
              title={t('preview.toolbar.openInTab')}
            />
            <IconButtonGroupItem
              icon={ArrowClockwiseIcon}
              onClick={onRefresh}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.refresh')}
              title={t('preview.toolbar.refresh')}
            />
          </IconButtonGroup>

          {/* Screen Size Toggle */}
          <IconButtonGroup>
            <IconButtonGroupItem
              icon={MonitorIcon}
              onClick={() => onScreenSizeChange('desktop')}
              active={screenSize === 'desktop'}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.desktopView')}
              title={t('preview.toolbar.desktopView')}
            />
            <IconButtonGroupItem
              icon={DeviceMobileIcon}
              onClick={() => onScreenSizeChange('mobile')}
              active={screenSize === 'mobile'}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.mobileView')}
              title={t('preview.toolbar.mobileView')}
            />
            <IconButtonGroupItem
              icon={ArrowsOutCardinalIcon}
              onClick={() => onScreenSizeChange('responsive')}
              active={screenSize === 'responsive'}
              disabled={!isServerRunning}
              aria-label={t('preview.toolbar.responsiveView')}
              title={t('preview.toolbar.responsiveView')}
            />
          </IconButtonGroup>

          {/* Dimensions display for responsive mode */}
          {screenSize === 'responsive' && (
            <span className="text-xs text-low font-mono whitespace-nowrap">
              {Math.round(localDimensions.width)} &times;{' '}
              {Math.round(localDimensions.height)}
            </span>
          )}

          {/* Start/Stop Button */}
          <IconButtonGroup>
            <IconButtonGroupItem
              icon={getServerControlIcon(isServerRunning, isStarting, isStopping)}
              iconClassName={
                shouldAnimateIcon(isServerRunning, isStarting, isStopping)
                  ? 'animate-spin'
                  : undefined
              }
              onClick={isServerRunning ? onStop : onStart}
              disabled={getServerControlDisabled(
                isServerRunning,
                isStarting,
                isStopping,
                hasDevScript
              )}
              aria-label={
                isServerRunning
                  ? t('preview.toolbar.stopDevServer')
                  : t('preview.toolbar.startDevServer')
              }
              title={
                isServerRunning
                  ? t('preview.toolbar.stopDevServer')
                  : t('preview.toolbar.startDevServer')
              }
            />
          </IconButtonGroup>
        </div>
      </div>

      {/* Content area */}
      <div
        ref={containerRef}
        className={cn(
          'flex-1 min-h-0 relative px-double pb-double',
          screenSize === 'mobile' ? 'overflow-hidden' : 'overflow-auto'
        )}
      >
        {showIframe ? (
          <div
            className={cn(
              'h-full',
              screenSize === 'desktop' ? '' : 'flex items-center justify-center'
            )}
          >
            {screenSize === 'mobile' ? (
              <MobileIframeView
                url={url}
                title={t('preview.browser.title')}
                mobileScale={mobileScale}
              />
            ) : (
              <DesktopIframeView
                url={url}
                title={t('preview.browser.title')}
                screenSize={screenSize}
                isResizing={isResizing}
                containerStyle={getIframeContainerStyle()}
                onResizeStart={onResizeStart}
                t={t}
              />
            )}
          </div>
        ) : (
          <div className="w-full h-full flex flex-col items-center justify-center gap-base text-low">
            <EmptyStateContent
              isLoading={isLoading}
              isStarting={isStarting}
              hasDevScript={hasDevScript}
              hasFailedDevServer={hasFailedDevServer}
              onStart={onStart}
              handleEditDevScript={handleEditDevScript}
              handleFixDevScript={handleFixDevScript}
              t={t}
            />
          </div>
        )}
      </div>
    </div>
  );
}
