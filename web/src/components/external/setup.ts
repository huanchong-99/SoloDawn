import type { MsgRow } from '../../lib/proto';
import prompt from './prompt.txt?raw';

export const checkText = 'Setup check: answer here in one line, then finish.';

export function setupPrompt(values: Record<'group' | 'name' | 'member' | 'exe', string>) {
  return prompt.replace(
    /\{(group|name|member|exe)\}/g,
    (_, field: keyof typeof values) => values[field],
  );
}

export function serverEntry(exe: string, member: string) {
  return JSON.stringify({ solodawn: { command: exe, args: ['mcp', '--member', member] } }, null, 2);
}

export function latestCheck(main: MsgRow[], owner: string | undefined, member: string) {
  return main.findLast(
    (msg) =>
      msg.kind === 'text' &&
      msg.by === owner &&
      msg.text === checkText &&
      msg.mentions.includes(member),
  );
}

export function checkReplies(main: MsgRow[], check: MsgRow, member: string) {
  return main.filter(
    (msg) => msg.pos > check.pos && msg.by === member && msg.kind === 'text' && msg.thread === null,
  );
}
