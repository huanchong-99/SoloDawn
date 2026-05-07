import { useMemo } from 'react';
import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { QualityRun } from 'shared/types';

export interface QualityTimelineProps {
  runs?: QualityRun[];
  className?: string;
}

function getStepCircleClass(status: string, stepId: string): string {
  if (status === 'completed') return "border-green-500 text-green-500";
  if (status === 'current' && stepId === 'feedback') return "border-amber-500 text-amber-500 bg-amber-50 dark:bg-amber-950/20 shadow-[0_0_0_4px_rgba(245,158,11,0.1)]";
  if (status === 'current') return "border-blue-500 text-blue-500 bg-blue-50 dark:bg-blue-950/20 shadow-[0_0_0_4px_rgba(59,130,246,0.1)] animate-pulse";
  return "border-slate-200 dark:border-slate-800 text-slate-300 dark:text-slate-700";
}

function getStepLabelClass(status: string, stepId: string): string {
  if (status === 'completed') return "text-slate-600 dark:text-slate-400";
  if (status === 'current' && stepId === 'feedback') return "text-amber-600 dark:text-amber-500";
  if (status === 'current') return "text-blue-600 dark:text-blue-500";
  return "text-slate-400 dark:text-slate-600";
}

const STEPS = [
  { id: 'checkpoint', label: 'Checkpoint' },
  { id: 'analysis', label: 'Analysis' },
  { id: 'feedback', label: 'Feedback' },
  { id: 'passed', label: 'Passed' },
];

export function QualityTimeline({ runs, className }: Readonly<QualityTimelineProps>) {
  const steps = STEPS;

  const currentStep = useMemo(() => {
    if (runs && runs.length > 0) {
      const latestRun = runs[0];
      if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
        return 'analysis';
      } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
        return 'feedback';
      } else if (latestRun.gateStatus === 'ok') {
        return 'passed';
      }
    }
    return 'checkpoint';
  }, [runs]);

  const stepStatuses = useMemo(() => {
    const currentIndex = steps.findIndex(s => s.id === currentStep);
    const map: Record<string, 'completed' | 'current' | 'pending'> = {};
    steps.forEach((s, idx) => {
      if (idx < currentIndex) map[s.id] = 'completed';
      else if (idx === currentIndex) map[s.id] = 'current';
      else map[s.id] = 'pending';
    });
    return map;
  }, [currentStep, steps]);

  const getStepStatus = (stepId: string) => stepStatuses[stepId] ?? 'pending';

  return (
    <div className={cn("relative", className)}>
      <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
      <div className="relative flex justify-between items-center w-full">
        {steps.map((step) => {
          const status = getStepStatus(step.id);
          
          return (
            <div key={step.id} className="relative z-10 flex flex-col items-center">
              <div 
                className={cn(
                  "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
                  getStepCircleClass(status, step.id)
                )}
              >
                {status === 'completed' && <CheckCircle className="w-4 h-4" />}
                {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
                {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
                {status === 'pending' && <Circle className="w-3 h-3" />}
              </div>
              
              <div className={cn(
                "absolute top-10 text-[10px] font-semibold tracking-wider uppercase whitespace-nowrap",
                getStepLabelClass(status, step.id)
              )}>
                {step.label}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
