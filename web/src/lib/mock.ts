// A ctl connection answered from the wire examples in internal/proto/testdata, so that screens can
// be built with no node. Development only: import it dynamically behind `import.meta.env.DEV`, so
// that the production build leaves it and the examples out.
import type { GenericSchema } from 'valibot';
import * as p from './proto';

const files = import.meta.glob<string>('../../../internal/proto/testdata/*.json', {
  query: '?raw',
  import: 'default',
  eager: true,
});

/** The text of each wire example, by file name without `.json`. */
export const goldens: Record<string, string> = Object.fromEntries(
  Object.entries(files).map(([path, text]) => [path.split('/').pop()!.slice(0, -5), text]),
);

// crypto.json holds signature test vectors for identity.ts, not a message.
export const examples = Object.keys(goldens).filter((name) => name !== 'crypto');

/** The schema of each wire example, by file name. */
export const schemaOf: Record<string, GenericSchema> = {
  answer: p.Answer,
  cmd: p.Cmd,
  'cmd-error': p.ErrorResponse,
  'cmd-ok': p.CmdResult,
  'cmd-pair-link': p.CmdResult,
  'cmd-reviewed': p.CmdResult,
  'cmd-started': p.CmdResult,
  'device-add': p.DeviceEntry,
  'device-revoke': p.DeviceEntry,
  devlog: p.Devlog,
  envelope: p.Envelope,
  fetch: p.FetchThread,
  'fetch-kind': p.FetchKind,
  'fetch-main': p.FetchThread,
  fetched: p.Fetched,
  hello: p.Hello,
  ice: p.ICECandidate,
  offer: p.Offer,
  pair: p.Pair,
  reset: p.Reset,
  'row-device': p.DeviceRow,
  'row-gone': p.GoneRow,
  'row-group': p.GroupRow,
  'row-group-paused': p.GroupRow,
  'row-member-ai': p.AIMemberRow,
  'row-member-human': p.HumanMemberRow,
  'row-msg': p.MsgRow,
  'row-note': p.NoteRow,
  'row-task': p.TaskRow,
  'row-task-waiting': p.TaskRow,
  rows: p.Rows,
  sync: p.Sync,
  synced: p.Synced,
  welcome: p.Welcome,
};

const answers = { hello: 'welcome', sync: 'synced', fetch: 'fetched', cmd: 'cmd-ok' } as const;

/** A connection to a node that knows the examples' one group. `sync` pushes `reset` and `rows`,
 *  then answers `synced`; every command is accepted. Frames pass through `decode` as real ones do. */
export function mockCtl(onpush: (push: p.Push) => void): p.Ctl {
  let id = 0;
  return {
    async request(req: { t: keyof typeof answers }) {
      id += 1;
      if (req.t === 'sync') {
        onpush(p.decode(p.Frame, goldens.reset) as p.Push);
        onpush(p.decode(p.Frame, goldens.rows) as p.Push);
      }
      return { ...p.decode(p.Frame, goldens[answers[req.t]]), re: id };
    },
  } as p.Ctl;
}
