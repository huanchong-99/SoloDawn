import type { AnswerTo, ErrorResponse, Hello } from './proto';

export async function hello(
  send: (req: Omit<Hello, 'id'>) => Promise<AnswerTo<'hello'> | ErrorResponse>,
  name: string,
) {
  const request = { t: 'hello' as const, kind: 'browser', name, addrs: [] };
  const answer = await send({ ...request, v: 2 });
  if (answer.err?.code === 'version') return send({ ...request, v: 1 });
  return answer;
}
