import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Terminal, ChevronDown } from 'lucide-react';
import ProcessLogsViewer from '../ProcessLogsViewer';
import { getDevServerWorkingDir } from '@/lib/devServerUtils';
import { cn } from '@/lib/utils';
import { ExecutionProcess } from 'shared/types';

interface DevServerLogsViewProps {
  devServerProcesses: ExecutionProcess[];
  showLogs: boolean;
  onToggle: () => void;
  height?: string;
  showToggleText?: boolean;
}

export function DevServerLogsView({
  devServerProcesses,
  showLogs,
  onToggle,
  height = 'h-60',
  showToggleText = true,
}: Readonly<DevServerLogsViewProps>) {
  const { t } = useTranslation('tasks');
  const [activeProcessId, setActiveProcessId] = useState<string | null>(null);

  useEffect(() => {
    if (devServerProcesses.length === 0) {
      if (activeProcessId !== null) setActiveProcessId(null);
      return;
    }
    // L14: also reset when the previously-active process is no longer in the
    // list (e.g. it was terminated and pruned). This avoids rendering logs for
    // a stale id that no longer matches any process.
    const stillPresent = devServerProcesses.some((p) => p.id === activeProcessId);
    if (!activeProcessId || !stillPresent) {
      setActiveProcessId(devServerProcesses[0].id);
    }
  }, [devServerProcesses, activeProcessId]);

  if (devServerProcesses.length === 0) {
    return null;
  }

  const activeProcess =
    devServerProcesses.find((p) => p.id === activeProcessId) ??
    devServerProcesses[0];

  return (
    <details
      className="group border-t bg-background"
      open={showLogs}
      onToggle={(e) => {
        if (e.currentTarget.open !== showLogs) {
          onToggle();
        }
      }}
    >
      <summary className="list-none cursor-pointer">
        <div className="flex items-center justify-between px-3 py-2 border-b bg-muted/50">
          <div className="flex items-center gap-2">
            <Terminal className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm font-medium text-foreground">
              {t('preview.logs.title')}
            </span>
          </div>
          <div className="flex items-center text-sm">
            <ChevronDown
              className={`h-4 w-4 mr-1 ${showToggleText ? 'transition-transform' : ''} ${showLogs ? '' : 'rotate-180'}`}
            />
            {(() => {
              if (!showToggleText) {
                return t('preview.logs.hide');
              }
              return showLogs ? t('preview.logs.hide') : t('preview.logs.show');
            })()}
          </div>
        </div>
      </summary>

      {showLogs && (
        <div className={height}>
          {devServerProcesses.length > 1 && (
            <div className="flex border-b bg-muted/30">
              {devServerProcesses.map((process) => (
                <button
                  key={process.id}
                  className={cn(
                    'px-3 py-1.5 text-sm border-b-2 transition-colors',
                    activeProcessId === process.id
                      ? 'border-primary text-foreground'
                      : 'border-transparent text-muted-foreground hover:text-foreground'
                  )}
                  onClick={() => setActiveProcessId(process.id)}
                >
                  {getDevServerWorkingDir(process) ?? 'Dev Server'}
                </button>
              ))}
            </div>
          )}
          <ProcessLogsViewer processId={activeProcess.id} />
        </div>
      )}
    </details>
  );
}
