import { QualityIssueRecord } from 'shared/types';
import { CheckCircle, AlertTriangle, StopCircle, Info, Bug, ChevronRight, ChevronDown } from 'lucide-react';
import { useState } from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

export interface QualityIssueListProps {
  issues: QualityIssueRecord[];
  className?: string;
  maxHeight?: string;
}

const SeverityIcon = ({ severity, className }: { severity: string, className?: string }) => {
  switch (severity.toLowerCase()) {
    case 'blocker':
    case 'critical':
      return <StopCircle className={cn('text-red-500 w-4 h-4', className)} />;
    case 'major':
      return <AlertTriangle className={cn('text-amber-500 w-4 h-4', className)} />;
    case 'minor':
      return <Bug className={cn('text-yellow-500 w-4 h-4', className)} />;
    case 'info':
    default:
      return <Info className={cn('text-blue-500 w-4 h-4', className)} />;
  }
};

const IssueItem = ({ issue }: { issue: QualityIssueRecord }) => {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border border-slate-200 dark:border-slate-800 rounded-md overflow-hidden bg-white dark:bg-slate-900 transition-all hover:border-slate-300 dark:hover:border-slate-700">
      <button
        type="button"
        className="flex items-start p-3 cursor-pointer gap-3 w-full text-left bg-transparent border-none"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="mt-0.5">
          <SeverityIcon severity={issue.severity} />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <h4 className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">
              {issue.message}
            </h4>
            <div className="flex-shrink-0 flex items-center gap-1.5 text-xs font-mono text-slate-500">
              <span className="font-medium text-slate-800 dark:text-slate-200">
                {issue.ruleId}
              </span>
            </div>
          </div>
          <div className="mt-1 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-2">
            <span className="truncate max-w-[200px] md:max-w-xs">{issue.filePath || 'Unknown file'}</span>
            {issue.line !== null && issue.line !== undefined && (
              <span>Line {issue.line.toString()}</span>
            )}
            <span className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded capitalize text-[10px]">
              {issue.source}
            </span>
          </div>
        </div>
        <Button variant="ghost" size="icon" className="w-6 h-6 shrink-0 mt-0.5" aria-expanded={expanded}>
          {expanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
        </Button>
      </button>
      {/* Removed context view because it is not provided by the DTO */}
    </div>
  );
};

export function QualityIssueList({ issues, className, maxHeight = '400px' }: Readonly<QualityIssueListProps>) {
  if (!issues || issues.length === 0) {
    return (
      <div className={cn("flex flex-col items-center justify-center p-8 text-slate-500 bg-slate-50 dark:bg-slate-900/50 rounded-lg border border-dashed border-slate-200 dark:border-slate-800", className)}>
        <CheckCircle className="w-8 h-8 text-green-500 mb-2" />
        <p className="text-sm font-medium">No quality issues found!</p>
        <p className="text-xs text-slate-400">Excellent job writing clean code.</p>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col gap-2 overflow-y-auto pr-1", className)} style={{ maxHeight }}>
      {issues.map(issue => (
        <IssueItem key={issue.id} issue={issue} />
      ))}
    </div>
  );
}
