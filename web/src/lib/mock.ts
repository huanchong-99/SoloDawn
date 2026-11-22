// A ctl connection answered from the wire examples in internal/proto/testdata, so that screens can
// be built with no node. Development only: import it dynamically behind `import.meta.env.DEV`, so
// that the production build leaves it and the examples out.
import type { GenericSchema } from 'valibot';
import * as p from './proto';

const files = import.meta.glob<string>('@testdata/*.json', {
  query: '?raw',
  import: 'default',
  eager: true,
});

/** The text of each wire example, by file name without `.json`. */
export const goldens: Record<string, string> = Object.fromEntries(
  Object.entries(files).map(([path, text]) => [path.split('/').pop()!.slice(0, -5), text]),
);

// crypto.json holds signature test vectors for identity.ts, tools.json the MCP tool lists of each
// kind of session, and wire.json what the two readers of the wire agree on, for proto.test.ts:
// none is a message.
export const examples = Object.keys(goldens).filter(
  (name) => !['crypto', 'tools', 'wire'].includes(name),
);

/** The schema of each wire example, by file name. */
export const schemaOf: Record<string, GenericSchema> = {
  ack: p.Ack,
  'act-approve': p.Act,
  'act-choose': p.Act,
  'act-confirm': p.Act,
  'act-deny': p.Act,
  'act-revise': p.Act,
  'agent-set': p.AgentSet,
  answer: p.Answer,
  'audit-answer': p.AuditAnswer,
  'audit-request': p.Audit,
  beacon: p.Beacon,
  cmd: p.Cmd,
  'cmd-error': p.ErrorResponse,
  'cmd-invite-link': p.CmdResult,
  'cmd-noted': p.CmdResult,
  'cmd-ok': p.CmdResult,
  'cmd-pair-link': p.CmdResult,
  'cmd-reviewed': p.CmdResult,
  'cmd-started': p.CmdResult,
  'device-add': p.DeviceEntry,
  'device-revoke': p.DeviceEntry,
  devlog: p.Devlog,
  envelope: p.Envelope,
  fetch: p.FetchThread,
  'fetch-kb': p.FetchKind,
  'fetch-kind': p.FetchKind,
  'fetch-main': p.FetchThread,
  'fetch-preset': p.FetchKind,
  'fetch-row': p.FetchRow,
  fetched: p.Fetched,
  forget: p.Forget,
  'group-create': p.GroupCreate,
  'group-pause': p.GroupCommand,
  'group-resume': p.GroupCommand,
  'group-set': p.GroupSet,
  groups: p.Groups,
  hello: p.Hello,
  'home-add': p.HomeAdd,
  'host-info-answer': p.HostInfo,
  'host-info-push': p.HostInfoPush,
  'host-info-request': p.HostInfoRequest,
  'host-join': p.HostJoin,
  'host-leave': p.HostLeave,
  'host-offline': p.HostCommandOnly,
  'host-online': p.HostCommandOnly,
  ice: p.ICECandidate,
  inbox: p.Inbox,
  invite: p.Invite,
  'item-assign': p.Item,
  'item-checks': p.Item,
  'item-confirmed': p.Item,
  'item-decided': p.Item,
  'item-mention': p.Item,
  'item-merge': p.Item,
  'item-notice': p.Item,
  'item-reply': p.Item,
  'item-resume': p.Item,
  'item-review': p.Item,
  'item-reviewed': p.Item,
  join: p.Join,
  jump: p.Jump,
  'member-add_ai': p.MemberAddAI,
  'member-remove': p.MemberCommand,
  'member-set': p.MemberSet,
  'member-set-role': p.MemberSet,
  'member-status': p.MemberStatus,
  'member-status-cap_reached': p.MemberStatus,
  'member-status-login_invalid': p.MemberStatus,
  'member-status-rate_limited': p.MemberStatus,
  'member-status-session_replaced': p.MemberStatus,
  'member-status-turn_interrupted': p.MemberStatus,
  note: p.Note,
  'note-kb': p.Note,
  'note-preset': p.Note,
  offer: p.Offer,
  pair: p.Pair,
  'pair-browser': p.HostCommandOnly,
  'perm-set': p.PermSet,
  'post-approval': p.Post,
  'post-brief': p.Post,
  'post-call': p.Post,
  'post-confirm': p.Post,
  'post-decision': p.Post,
  'post-handback': p.Post,
  'post-progress': p.Post,
  'post-question': p.Post,
  'post-reply': p.Post,
  'post-report': p.Post,
  'post-review': p.Post,
  'post-summary': p.Post,
  'post-text': p.Post,
  'pty-resize': p.PTYResize,
  'push-off': p.PushOff,
  'push-on': p.PushOn,
  'push-reminder': p.Reminder,
  'push-reminder-confirm': p.Reminder,
  reset: p.Reset,
  'row-device': p.DeviceRow,
  'row-gone': p.GoneRow,
  'row-group': p.GroupRow,
  'row-group-paused': p.GroupRow,
  'row-member-ai': p.AIMemberRow,
  'row-member-human': p.HumanMemberRow,
  'row-msg-approval-approved': p.MsgRow,
  'row-msg-approval-denied': p.MsgRow,
  'row-msg-approval-expired': p.MsgRow,
  'row-msg-approval-pending': p.MsgRow,
  'row-msg-approval-reassigned': p.MsgRow,
  'row-msg-brief-confirmed': p.MsgRow,
  'row-msg-brief-pending': p.MsgRow,
  'row-msg-brief-preset': p.MsgRow,
  'row-msg-brief-revise': p.MsgRow,
  'row-msg-brief-superseded': p.MsgRow,
  'row-msg-call-answered': p.MsgRow,
  'row-msg-call-denied': p.MsgRow,
  'row-msg-call-waiting': p.MsgRow,
  'row-msg-checks': p.MsgRow,
  'row-msg-confirm-confirmed': p.MsgRow,
  'row-msg-confirm-denied': p.MsgRow,
  'row-msg-confirm-expired': p.MsgRow,
  'row-msg-confirm-pending': p.MsgRow,
  'row-msg-decision-decided': p.MsgRow,
  'row-msg-decision-expired': p.MsgRow,
  'row-msg-decision-pending': p.MsgRow,
  'row-msg-handback': p.MsgRow,
  'row-msg-progress': p.MsgRow,
  'row-msg-question': p.MsgRow,
  'row-msg-reply': p.MsgRow,
  'row-msg-report': p.MsgRow,
  'row-msg-review': p.MsgRow,
  'row-msg-summary': p.MsgRow,
  'row-msg-system': p.MsgRow,
  'row-msg-task': p.MsgRow,
  'row-msg-text': p.MsgRow,
  'row-note': p.NoteRow,
  'row-note-kb': p.NoteRow,
  'row-note-preset': p.NoteRow,
  'row-task': p.TaskRow,
  'row-task-confirm': p.TaskRow,
  'row-task-paused': p.TaskRow,
  'row-task-waiting': p.TaskRow,
  rows: p.Rows,
  'rv-auth': p.RVAuth,
  'rv-challenge': p.RVChallenge,
  'rv-from': p.RVFrom,
  'rv-ice-answer': p.RVICEAnswer,
  'rv-ice-request': p.RVICERequest,
  'rv-join': p.RVJoin,
  'rv-joined': p.RVJoined,
  'rv-offline': p.RVOffline,
  'rv-presence': p.RVPresence,
  'rv-quota-answer': p.RVQuotaAnswer,
  'rv-quota-request': p.RVQuotaRequest,
  'rv-ready': p.RVReady,
  'rv-room': p.RVRoom,
  'rv-to': p.RVTo,
  'rv-watch': p.RVWatch,
  'settings-set': p.SettingsSet,
  sync: p.Sync,
  synced: p.Synced,
  'takeover-end': p.TakeoverEnd,
  'takeover-start': p.TakeoverStart,
  'task-assign': p.TaskAssign,
  'task-branch': p.TaskBranch,
  'task-cancel': p.TaskCommand,
  'task-checked': p.TaskChecked,
  'task-cloud': p.TaskChecked,
  'task-conflict': p.TaskConflict,
  'task-fail': p.TaskFail,
  'task-merged': p.TaskMerged,
  'task-pause': p.TaskCommand,
  'task-preempted': p.TaskPreempted,
  'task-resume': p.TaskCommand,
  'task-start': p.TaskStart,
  'task-submitted': p.TaskSubmitted,
  welcome: p.Welcome,
};

/** The example that answers each request. */
const answers = {
  hello: 'welcome',
  sync: 'synced',
  fetch: 'fetched',
  cmd: 'cmd-ok',
  'host.info': 'host-info-answer',
  audit: 'audit-answer',
} as const satisfies Record<p.RequestType, string>;

type CmdFields = { t: string; op?: string; type?: string };

const cmdAnswers: Record<string, string> = {
  invite: 'cmd-invite-link',
  'pair.browser': 'cmd-pair-link',
};

/** Whether a command adds a readable group, after which a node pushes `groups`. */
function pushesGroups({ t, op }: CmdFields): boolean {
  return t === 'join' || (t === 'group' && op === 'create');
}

/** Which golden command answer a node sends for a signed command payload. */
function cmdAnswer({ t, type }: CmdFields): string {
  if (t === 'note' && type !== undefined) return 'cmd-noted';
  return cmdAnswers[t] ?? 'cmd-ok';
}

/** A connection to a node that knows the examples' one group and is the owner's: it answers
 *  `host.info` and `audit`. `sync` pushes `reset` and `rows`, then answers `synced`; commands
 *  answer with their typed example (`cmd-invite-link`, `cmd-pair-link`, `cmd-noted`, or `cmd-ok`),
 *  and `group create` or `join` pushes `groups` before its answer. Frames are read as real ones
 *  are: answers with `decodeAnswer`, pushes with `Frame`. */
export function mockCtl(onpush: (push: p.Push) => void): p.Ctl {
  let id = 0;
  return {
    async request(req: { t: keyof typeof answers; env?: p.Envelope }) {
      id += 1;
      const push = (name: string) => onpush(p.decode(p.Frame, goldens[name]) as p.Push);
      if (req.t === 'sync') {
        push('reset');
        push('rows');
      }
      let example: string = answers[req.t];
      if (req.t === 'cmd') {
        const fields = JSON.parse(req.env!.p) as CmdFields;
        if (pushesGroups(fields)) push('groups');
        example = cmdAnswer(fields);
      }
      return { ...p.decodeAnswer(req.t, goldens[example]), re: id };
    },
  } as p.Ctl;
}
