// Standalone verification of both signed task transcripts (flows/trace-task.md).
import * as v from 'valibot';
import { describe, expect, it } from 'vitest';
import { DeviceLog } from './devlog';
import { openCommand, verifyEnvelope } from './identity';
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

async function verifyLog(trace: p.TaskTrace) {
  const log = await DeviceLog.replay(crypto, trace.devlog.user, trace.devlog.entries);
  const valid = new Set(log.validDevices());
  expect(valid).toEqual(new Set(trace.keys.map((k) => k.device)));
  return valid;
}

function verifyCast(trace: p.TaskTrace, valid: Set<string>, remote: boolean) {
  expect(trace.group.forge).toBe(remote ? 'github' : 'none');
  expect(trace.owner.user).toBe(trace.devlog.user);
  expect(valid.has(trace.group.home)).toBe(true);
  const ais = new Map(trace.ais.map((ai) => [ai.id, ai]));
  expect(ais.size).toBe(3);
  for (const ai of ais.values()) expect(valid.has(ai.host)).toBe(true);
  const brief = trace.steps
    .map((step) => JSON.parse(step.cmd.env.p))
    .find((cmd) => cmd.kind === 'brief');
  const reviewer = ais.get(brief.body.tasks[0].reviewer)!;
  expect(reviewer.secretary).toBe(false);
  expect(reviewer.host === trace.group.home).toBe(!remote);
  return ais;
}

function cmdKey(raw: string) {
  const parsed = JSON.parse(raw) as { t: string; ts: number; op?: string; as?: string };
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
  const opened = await openCommand(crypto, step.cmd.env, '', (d) => valid.has(d), parsed.ts);
  expect('header' in opened).toBe(true);
  expect(p.decode(cmdSchemas[key], step.cmd.env.p)).toMatchObject({ g: trace.group.id });
  const expectedHost = parsed.as ? ais.get(parsed.as)?.host : step.from;
  expect(expectedHost).toBe(step.from);
  const ans = p.decodeAnswer('cmd', JSON.stringify(step.answer));
  expect(ans).toStrictEqual(step.answer);
  expect(step.answer.re).toBe(step.cmd.id);
}

interface TraceState {
  seq: number;
  confirmEnv: p.Envelope | undefined;
  msgs: Map<string, p.MsgRow>;
  tasks: Map<string, p.TaskRow>;
  whys: string[];
  nextItem: number;
}

async function verifyStepRows(step: p.TaskTrace['steps'][number], state: TraceState) {
  expect(p.decode(p.Frame, JSON.stringify(step.rows))).toStrictEqual(step.rows);
  const parsedCmd = JSON.parse(step.cmd.env.p) as { t: string; action?: string };
  if (`${parsedCmd.t}/${parsedCmd.action}` === 'act/confirm') state.confirmEnv = step.cmd.env;
  for (const row of step.rows.rows) {
    await applyStepRow(row, step.cmd.env, state);
  }
}

async function applyStepRow(row: p.Row, cmdEnv: p.Envelope, state: TraceState) {
  state.seq += 1;
  expect(row.seq).toBe(state.seq);
  expect('gone' in row).toBe(false);
  if ('gone' in row) return;
  if (row.r === 'task') state.tasks.set(row.id, row);
  if (row.r === 'msg') await verifyMsgRow(row, cmdEnv, state);
}

async function verifyMsgRow(row: p.MsgRow, cmdEnv: p.Envelope, state: TraceState) {
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

/** An item carries the rows it names as they stand. */
function verifyItemRows(entry: p.Inbox['items'][number], state: TraceState) {
  const task = state.tasks.get(entry.item.task ?? '');
  expect(entry.task).toStrictEqual(task);
  expect(entry.msg).toStrictEqual(state.msgs.get(entry.item.msg ?? ''));
  verifyItemCommit(entry.item, task);
}

/** An item's commit and pull request are the task's: the commit that was submitted. */
function verifyItemCommit(item: p.Item, task: p.TaskRow | undefined) {
  if (item.commit) expect(item.commit).toBe(task?.commit);
  if (item.pr) expect(item.pr).toBe(task?.pr);
}

function verifyStepInboxes(
  inboxes: p.TaskTrace['steps'][number]['inboxes'],
  group: string,
  ais: Map<string, p.AIMemberRow>,
  state: TraceState,
) {
  for (const delivery of inboxes) {
    expect(delivery.inbox.g).toBe(group);
    expect(delivery.acks.length).toBe(delivery.inbox.items.length);
    delivery.inbox.items.forEach((entry, idx) => {
      state.nextItem += 1;
      expect(ais.get(entry.item.ai)?.host).toBe(delivery.to);
      expect(entry.item.item).toBe(state.nextItem);
      expect(delivery.acks[idx]).toStrictEqual({ t: 'ack', g: group, item: state.nextItem });
      verifyItemRows(entry, state);
      state.whys.push(entry.item.why);
    });
  }
}

function verifyFinalOutcome(trace: p.TaskTrace, state: TraceState, remote: boolean) {
  expect(state.whys).toEqual(['mention', 'assign', 'confirmed', 'review', 'merge', 'notice']);
  expect(trace.steps.length).toBe(remote ? 12 : 11);
  expect(state.tasks.size).toBe(1);
  const [task] = [...state.tasks.values()];
  expect([task.num, task.state, task.step]).toEqual([12, 'merged', 'merge']);
  expect(task.pr !== undefined).toBe(remote);
  expect(task.checks.cloud !== undefined).toBe(remote);
  const mergedSys = [...state.msgs.values()].find((m) => m.kind === 'system');
  if (mergedSys?.kind !== 'system' || mergedSys.body.code !== 'task_merged')
    throw new Error('no task_merged message');
  expect(mergedSys.body.commit).toBe(task.commit);
  expect(mergedSys.body.merge_commit).not.toBe(task.commit);
  const summary = [...state.msgs.values()].find((m) => m.kind === 'summary');
  expect(summary?.kind === 'summary' ? summary.body.pr !== undefined : false).toBe(remote);
}

describe('signed task transcripts', () => {
  it.each([
    ['trace-remote', true],
    ['trace-local', false],
  ] as const)('%s parses alone and verifies end to end', async (name, remote) => {
    const trace = p.decode(p.TaskTrace, goldens[name]);
    const valid = await verifyLog(trace);
    const ais = verifyCast(trace, valid, remote);
    const state: TraceState = {
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
