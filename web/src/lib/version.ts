import type { AnswerTo, ErrorResponse, Hello } from './proto';

/** Says `hello` with protocol version 3, the one there is: a node that answers `version` must be
 *  updated together with this client, and is never asked again with a lower one
 *  (contracts/datachannel.md section 3). */
export const hello = (
  send: (req: Omit<Hello, 'id'>) => Promise<AnswerTo<'hello'> | ErrorResponse>,
  name: string,
) => send({ t: 'hello', v: 3, kind: 'browser', name, addrs: [] });
