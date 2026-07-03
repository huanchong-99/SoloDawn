import { describe, it, expect } from 'vitest';
import enTasks from '../locales/en/tasks.json';
import zhHansTasks from '../locales/zh-Hans/tasks.json';
import zhHantTasks from '../locales/zh-Hant/tasks.json';
import jaTasks from '../locales/ja/tasks.json';
import koTasks from '../locales/ko/tasks.json';
import esTasks from '../locales/es/tasks.json';
import enSettings from '../locales/en/settings.json';
import zhHansSettings from '../locales/zh-Hans/settings.json';
import zhHantSettings from '../locales/zh-Hant/settings.json';
import jaSettings from '../locales/ja/settings.json';
import koSettings from '../locales/ko/settings.json';
import esSettings from '../locales/es/settings.json';

const TASKS_LOCALES: Record<string, unknown> = {
  en: enTasks,
  'zh-Hans': zhHansTasks,
  'zh-Hant': zhHantTasks,
  ja: jaTasks,
  ko: koTasks,
  es: esTasks,
};

const SETTINGS_LOCALES: Record<string, unknown> = {
  en: enSettings,
  'zh-Hans': zhHansSettings,
  'zh-Hant': zhHantSettings,
  ja: jaSettings,
  ko: koSettings,
  es: esSettings,
};

function lookup(obj: unknown, path: string): unknown {
  return path
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object'
          ? (node as Record<string, unknown>)[seg]
          : undefined,
      obj
    );
}

// Every key the planner/ledger UI consumes must exist in every locale —
// a missing key silently renders as its raw path.
const TASKS_KEYS = [
  'conversation.planning.confirmButton',
  'conversation.planning.generatingRubric',
  'conversation.planning.rounds.badge',
  'conversation.planning.rounds.divider',
  'conversation.planning.rounds.continueButton',
  'conversation.planning.auditPlan.cardTitle',
  'conversation.planning.auditPlan.cardSummary',
  'conversation.planning.auditPlan.maxScore',
  'conversation.planning.ledger.panelTab',
  'conversation.planning.ledger.panelTitle',
  'conversation.planning.ledger.progress',
  'conversation.planning.ledger.status.pending',
  'conversation.planning.ledger.status.delivered',
  'conversation.planning.ledger.status.regressed',
  'conversation.planning.ledger.capsuleBuilt',
  'conversation.planning.ledger.capsuleLivesWhere',
  'conversation.planning.ledger.capsuleDecisions',
  'conversation.planning.ledger.capsuleExtensionNotes',
  'conversation.planning.ledger.capsuleProvenance',
  'conversation.planning.ledger.showCapsule',
  'conversation.planning.ledger.hideCapsule',
  'conversation.planning.ledger.edit',
  'conversation.planning.ledger.save',
  'conversation.planning.ledger.cancel',
  'conversation.planning.ledger.delete',
];

const SETTINGS_KEYS = [
  'qualityGates.confirmTitle',
  'qualityGates.confirmDescription',
  'qualityGates.saveAndConfirm',
  'qualityGates.confirmError',
];

describe('requirement ledger & rounds i18n keys', () => {
  it.each(Object.keys(TASKS_LOCALES))(
    'tasks.json (%s) defines every planner/ledger key',
    (locale) => {
      for (const key of TASKS_KEYS) {
        const value = lookup(TASKS_LOCALES[locale], key);
        expect(value, `${locale}: missing ${key}`).toBeTypeOf('string');
        expect((value as string).length, `${locale}: empty ${key}`).toBeGreaterThan(0);
      }
    }
  );

  it.each(Object.keys(SETTINGS_LOCALES))(
    'settings.json (%s) defines the quality-gate dialog keys',
    (locale) => {
      for (const key of SETTINGS_KEYS) {
        const value = lookup(SETTINGS_LOCALES[locale], key);
        expect(value, `${locale}: missing ${key}`).toBeTypeOf('string');
      }
    }
  );

  it('interpolation placeholders are consistent across locales', () => {
    for (const locale of Object.keys(TASKS_LOCALES)) {
      expect(
        lookup(TASKS_LOCALES[locale], 'conversation.planning.rounds.badge')
      ).toContain('{{round}}');
      const summary = lookup(
        TASKS_LOCALES[locale],
        'conversation.planning.auditPlan.cardSummary'
      ) as string;
      expect(summary).toContain('{{points}}');
      expect(summary).toContain('{{threshold}}');
      const progress = lookup(
        TASKS_LOCALES[locale],
        'conversation.planning.ledger.progress'
      ) as string;
      expect(progress).toContain('{{delivered}}');
      expect(progress).toContain('{{total}}');
    }
  });
});
