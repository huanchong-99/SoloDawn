import { ExecutionProcess } from 'shared/types';

export type AttemptData = {
  processes: ExecutionProcess[];
  runningProcessDetails: Record<string, ExecutionProcess>;
};

