// Standalone verification of both signed task transcripts (flows/trace-task.md).
import * as v from 'valibot';
import { describe, expect, it } from 'vitest';
import { DeviceLog } from './devlog';
import { Device, decode, openCommand, verifyEnvelope } from './identity';
import { goldens } from './mock';
import * as p from './proto';

const cmdSchemas: Record<string, v.GenericSchema> = {
  post: p.Post,
  act: p.Act,
  'task/branch': p.TaskBranch,
  'task/checked': p.TaskChecked,
  'task/submitted': p.TaskSubmitted,
  'task/cloud': p.TaskChecked,
  'task/merged': p.TaskMerged,
};

async function importSeedDevice(seed: string, id: string): Promise<Device> {
  const pkcs8 = new Uint8Array([
    ...[0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04],
    ...[0x20, ...decode(seed)!],
  ]);
  const privateKey = await crypto.subtle.importKey('pkcs8', pkcs8, 'Ed25519', false, ['sign']);
  const publicKey = await crypto.subtle.importKey('raw', decode(id)!, 'Ed25519', true, ['verify']);
  return Device.of(crypto, { privateKey, publicKey });
}

async function verifyKeysAndLog(trace: p.TaskTrace, remote: boolean) {
  expect(trace.keys.length).toBe(remote ? 3 : 2);
  for (const k of trace.keys) {
    const dev = await importSeedDevice(k.seed, k.device);
    expect(dev.id).toBe(k.device);
    expect(await verifyEnvelope(crypto, await dev.sign('trace key check'))).toBe(true);
  }
  const log = await DeviceLog.replay(crypto, trace.devlog.user, trace.devlog.entries);
  const valid = new Set(log.validDevices());
  expect(valid).toEqual(new Set(trace.keys.map((k) => k.device)));
  return valid;
}

function verifyCast(trace: p.TaskTrace, valid: Set<string>, remote: boolean) {
  expect(trace.group.forge).toBe(remote ? 'github' : 'none');
  expect(trace.owner.user).toBe(trace.devlog.user);
  expect(valid.has(trace.group.home)).toBe(true);
  expect([trace.seq, trace.num]).toEqual([800, 11]);
  const ais = new Map(trace.ais.map((ai) => [ai.id, ai]));
  expect(ais.size).toBe(3);
  for (const ai of ais.values()) expect(valid.has(ai.host)).toBe(true);
  const reviewer = trace.ais.find((ai) => ai.name === 'Codex')!;
  expect(reviewer.secretary).toBe(false);
  expect(reviewer.host === trace.group.home).toBe(!remote);
  return ais;
}

function cmdKey(raw: string) {
  const parsed = JSON.parse(raw) as { t: string; op?: string; g?: string; as?: string };
  return { key: parsed.op ? `${parsed.t}/${parsed.op}` : parsed.t, parsed };
}

async function verifyStepCmd(
  step: p.TaskTrace['steps'][number],
  trace: p.TaskTrace,
  valid: Set<string>,
  ais: Map<string, p.AIMemberRow>,
) {
  expect(valid.has(step.from)).toBe(true);
  expect(step.cmd.env.k).toBe(step.from);
  const { key, parsed } = cmdKey(step.cmd.env.p);
  const now = trace.group.created + 200000;
  const opened = await openCommand(crypto, step.cmd.env, '', (d) => valid.has(d), now);
  expect('header' in opened).toBe(true);
  expect(p.decode(cmdSchemas[key], step.cmd.env.p)).toMatchObject({ g: trace.group.id });
  const expectedHost = parsed.as ? ais.get(parsed.as)?.host : step.from;
  expect(expectedHost).toBe(step.from);
  const ans = p.decodeAnswer('cmd', JSON.stringify(step.answer));
  expect(ans).toStrictEqual(step.answer);
  expect(step.answer.re).toBe(step.cmd.id);
}

interface AuditState {
  seq: number;
  confirmEnv: p.Envelope | undefined;
  msgs: Map<string, p.MsgRow>;
  tasks: Map<string, p.TaskRow>;
  whys: string[];
  nextItem: number;
}

async function verifyStepRows(step: p.TaskTrace['steps'][number], state: AuditState) {
  expect(p.decode(p.Frame, JSON.stringify(step.rows))).toStrictEqual(step.rows);
  const parsedCmd = JSON.parse(step.cmd.env.p) as { t: string; action?: string };
  if (`${parsedCmd.t}/${parsedCmd.action}` === 'act/confirm') state.confirmEnv = step.cmd.env;
  for (const row of step.rows.rows) {
    await applyStepRow(row, step.cmd.env, state);
  }
}

async function applyStepRow(row: p.Row, cmdEnv: p.Envelope, state: AuditState) {
  state.seq += 1;
  expect(row.seq).toBe(state.seq);
  expect('gone' in row).toBe(false);
  if ('gone' in row) return;
  if (row.r === 'task') state.tasks.set(row.id, row);
  if (row.r === 'msg') await verifyMsgRow(row, cmdEnv, state);
}

async function verifyMsgRow(row: p.MsgRow, cmdEnv: p.Envelope, state: AuditState) {
  if (row.env) expect(await verifyEnvelope(crypto, row.env)).toBe(true);
  if ([row.by !== 'system', row.state === null].every(Boolean)) {
    expect(row.env).toStrictEqual(cmdEnv);
  }
  if (row.kind === 'brief' && row.state.status === 'confirmed') {
    expect(row.state.act).toStrictEqual(state.confirmEnv);
    expect(await verifyEnvelope(crypto, row.state.act)).toBe(true);
  }
  state.msgs.set(row.id, row);
}

function verifyStepInboxes(
  inboxes: p.TaskTrace['steps'][number]['inboxes'],
  group: string,
  ais: Map<string, p.AIMemberRow>,
  state: AuditState,
) {
  for (const delivery of inboxes) {
    expect(delivery.inbox.g).toBe(group);
    expect(delivery.acks.length).toBe(delivery.inbox.items.length);
    delivery.inbox.items.forEach((entry, idx) => {
      state.nextItem += 1;
      expect(ais.get(entry.item.ai)?.host).toBe(delivery.to);
      expect(entry.item.item).toBe(state.nextItem);
      expect(delivery.acks[idx]).toStrictEqual({ t: 'ack', g: group, item: state.nextItem });
      if (entry.item.task) expect(entry.task).toStrictEqual(state.tasks.get(entry.item.task));
      if (entry.item.msg) expect(entry.msg).toStrictEqual(state.msgs.get(entry.item.msg));
      state.whys.push(entry.item.why);
    });
  }
}

function verifyFinalOutcome(trace: p.TaskTrace, state: AuditState, remote: boolean) {
  expect(state.whys).toEqual(['mention', 'assign', 'confirmed', 'review', 'merge', 'notice']);
  expect(trace.steps.length).toBe(remote ? 12 : 11);
  expect(state.tasks.size).toBe(1);
  const [task] = [...state.tasks.values()];
  expect([task.num, task.state, task.step]).toEqual([12, 'merged', 'merge']);
  expect(task.pr !== undefined).toBe(remote);
  expect(task.checks.cloud !== undefined).toBe(remote);
  const mergedSys = [...state.msgs.values()].find((m) => m.kind === 'system');
  expect(mergedSys?.kind === 'system' ? mergedSys.body.code : '').toBe('task_merged');
  const summary = [...state.msgs.values()].find((m) => m.kind === 'summary');
  expect(summary?.kind === 'summary' ? summary.body.pr !== undefined : false).toBe(remote);
}

describe('signed task transcripts', () => {
  it.each([
    ['trace-remote', true],
    ['trace-local', false],
  ] as const)('%s parses alone and verifies end to end', async (name, remote) => {
    const trace = p.decode(p.TaskTrace, goldens[name]);
    const valid = await verifyKeysAndLog(trace, remote);
    const ais = verifyCast(trace, valid, remote);
    const state: AuditState = {
      seq: trace.seq,
      confirmEnv: undefined,
      msgs: new Map(),
      tasks: new Map(),
      whys: [],
      nextItem: 0,
    };
    for (const step of trace.steps) {
      await verifyStepCmd(step, trace, valid, ais);
      await verifyStepRows(step, state);
      verifyStepInboxes(step.inboxes, trace.group.id, ais, state);
    }
    verifyFinalOutcome(trace, state, remote);
  });
});
