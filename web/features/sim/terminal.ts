// The terminals of the demo world (contracts/datachannel.md sections 1 and 7, watch-team.md
// section 4): a `pty` channel of a takeover, whose CLI echoes what the person types and answers each
// line, and a `watch` channel that shows Mina's terminal to a person she let watch it. Both ends of
// a channel are in this page: what the client sends reaches the script, what the script writes
// reaches the client's `onmessage`.
import type * as p from '../../src/lib/proto';
import { banners, minaScreen, terminalReply } from './content';

const encode = (text: string) => new TextEncoder().encode(text);

/** The client's end of a channel whose other end is a script. */
export class Channel implements p.PtyChannel {
  binaryType: BinaryType = 'arraybuffer';
  readyState = 'open';
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  /** What the client sent, to the script. */
  heard: (bytes: Uint8Array) => void = () => {};
  /** The client closed it. */
  ended: () => void = () => {};

  send(data: Uint8Array<ArrayBuffer>) {
    if (this.readyState === 'open') this.heard(data);
  }

  close() {
    if (this.readyState !== 'open') return;
    this.readyState = 'closed';
    this.ended();
  }

  /** The script writes to the client. */
  write(text: string) {
    if (this.readyState === 'open') this.onmessage?.({ data: encode(text).buffer });
  }

  /** The script closes the channel: the client hears it. */
  shut() {
    if (this.readyState !== 'open') return;
    this.readyState = 'closed';
    this.onclose?.();
    this.ended();
  }
}

const prompt = '\r\n\x1b[36m> \x1b[0m';

/** A takeover's CLI: it echoes what is typed, and answers a line when Enter is pressed. */
export function takeoverTerminal(cli: string): Channel {
  const channel = new Channel();
  let line = '';
  const keys: Record<string, () => void> = {
    '\r': () => {
      const said = line.trim();
      line = '';
      setTimeout(() => channel.write((said ? terminalReply(said) : '') + prompt), said ? 700 : 0);
    },
    '\x7f': () => {
      if (!line) return;
      line = line.slice(0, -1);
      channel.write('\b \b');
    },
    '\x03': () => {
      line = '';
      channel.write(`^C${prompt}`);
    },
  };
  channel.heard = (bytes) => {
    const text = new TextDecoder().decode(bytes);
    if (keys[text]) return keys[text]();
    if (text.startsWith('\x1b')) return;
    line += text;
    channel.write(text);
  };
  setTimeout(() => channel.write((banners[cli] ?? banners.claude) + prompt), 300);
  return channel;
}

/** Mina's terminal as a watcher sees it: a line every second, again from the top after a pause. */
export function watchedTerminal(): Channel {
  const channel = new Channel();
  let at = 0;
  const timer = setInterval(() => {
    if (at === minaScreen.length + 4) {
      at = 0;
      channel.write('\x1b[2J\x1b[H');
    }
    if (at < minaScreen.length) channel.write(`${minaScreen[at]}\r\n`);
    at += 1;
  }, 1100);
  channel.ended = () => clearInterval(timer);
  return channel;
}
