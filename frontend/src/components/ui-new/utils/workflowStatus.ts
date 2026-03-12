import i18next, { type TFunction } from 'i18next';
import type { Icon } from '@phosphor-icons/react';
import {
  WarningIcon,
  CheckCircleIcon,
  CircleIcon,
  ClockIcon,
  SpinnerIcon,
  PauseIcon,
  PlayIcon,
  XCircleIcon,
  SealQuestionIcon,
} from '@phosphor-icons/react';

export type StatusTone =
  | 'success'
  | 'warning'
  | 'info'
  | 'neutral'
  | 'danger'
  | 'brand';

export interface StatusMeta {
  label: string;
  tone: StatusTone;
  icon: Icon;
  spin?: boolean;
}

type StatusConfig = {
  key: string;
  tone: StatusTone;
  icon: Icon;
  spin?: boolean;
};

const WORKFLOW_STATUS_CONFIG: Record<string, StatusConfig> = {
  created: {
    key: 'workflow:status.created',
    tone: 'neutral',
    icon: CircleIcon,
  },
  ready: {
    key: 'workflow:status.ready',
    tone: 'info',
    icon: PlayIcon,
  },
  starting: {
    key: 'workflow:status.starting',
    tone: 'info',
    icon: SpinnerIcon,
    spin: true,
  },
  running: {
    key: 'workflow:status.running',
    tone: 'brand',
    icon: PlayIcon,
  },
  merging: {
    key: 'workflow:status.merging',
    tone: 'brand',
    icon: SpinnerIcon,
    spin: true,
  },
  paused: {
    key: 'workflow:status.paused',
    tone: 'warning',
    icon: PauseIcon,
  },
  completed: {
    key: 'workflow:status.completed',
    tone: 'success',
    icon: CheckCircleIcon,
  },
  failed: {
    key: 'workflow:status.failed',
    tone: 'danger',
    icon: XCircleIcon,
  },
  cancelled: {
    key: 'workflow:status.cancelled',
    tone: 'neutral',
    icon: XCircleIcon,
  },
  idle: {
    key: 'workflow:status.idle',
    tone: 'neutral',
    icon: CircleIcon,
  },
  queued: {
    key: 'workflow:status.queued',
    tone: 'info',
    icon: ClockIcon,
  },
  unknown: {
    key: 'workflow:status.unknown',
    tone: 'neutral',
    icon: CircleIcon,
  },
};

const TERMINAL_STATUS_CONFIG: Record<string, StatusConfig> = {
  not_started: {
    key: 'workflow:terminalDebug.status.not_started',
    tone: 'neutral',
    icon: CircleIcon,
  },
  starting: {
    key: 'workflow:terminalDebug.status.starting',
    tone: 'info',
    icon: SpinnerIcon,
    spin: true,
  },
  waiting: {
    key: 'workflow:terminalDebug.status.waiting',
    tone: 'info',
    icon: ClockIcon,
  },
  working: {
    key: 'workflow:terminalDebug.status.working',
    tone: 'brand',
    icon: PlayIcon,
  },
  running: {
    key: 'workflow:terminalDebug.status.running',
    tone: 'brand',
    icon: PlayIcon,
  },
  active: {
    key: 'workflow:terminalDebug.status.active',
    tone: 'brand',
    icon: PlayIcon,
  },
  paused: {
    key: 'workflow:terminalDebug.status.paused',
    tone: 'warning',
    icon: PauseIcon,
  },
  completed: {
    key: 'workflow:terminalDebug.status.completed',
    tone: 'success',
    icon: CheckCircleIcon,
  },
  failed: {
    key: 'workflow:terminalDebug.status.failed',
    tone: 'danger',
    icon: XCircleIcon,
  },
  killed: {
    key: 'workflow:terminalDebug.status.killed',
    tone: 'danger',
    icon: WarningIcon,
  },
  idle: {
    key: 'workflow:terminalDebug.status.idle',
    tone: 'neutral',
    icon: CircleIcon,
  },
  waiting_for_approval: {
    key: 'workflow:terminalDebug.status.waiting_for_approval',
    tone: 'warning',
    icon: SealQuestionIcon,
  },
  stalled: {
    key: 'workflow:terminalDebug.status.stalled',
    tone: 'warning',
    icon: WarningIcon,
  },
  unknown: {
    key: 'workflow:terminalDebug.status.unknown',
    tone: 'neutral',
    icon: CircleIcon,
  },
};

const defaultT = i18next.t.bind(i18next);

function normalizeStatus(status?: string | null) {
  return (status ?? 'unknown').toLowerCase();
}

function resolveStatusMeta(
  status: string | null | undefined,
  config: Record<string, StatusConfig>,
  t: TFunction
): StatusMeta {
  const normalized = normalizeStatus(status);
  const entry = config[normalized] ?? config.unknown;
  const label = t(entry.key, { defaultValue: status ?? 'Unknown' });

  return {
    label,
    tone: entry.tone,
    icon: entry.icon,
    spin: entry.spin,
  };
}

export function getWorkflowStatusMeta(
  status: string | null | undefined,
  t: TFunction = defaultT
): StatusMeta {
  return resolveStatusMeta(status, WORKFLOW_STATUS_CONFIG, t);
}

export function getTerminalStatusMeta(
  status: string | null | undefined,
  t: TFunction = defaultT
): StatusMeta {
  return resolveStatusMeta(status, TERMINAL_STATUS_CONFIG, t);
}
