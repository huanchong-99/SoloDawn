import { lost, type Group, type Home } from '../../lib/groups.svelte';
import type { Path } from '../../lib/net.svelte';
import type { AIMemberRow, HostInfoPush, PtyChannel } from '../../lib/proto';
import { toTerminalChunk, type Target, type Write } from '../../lib/terminal.svelte';

export function watchers(info: HostInfoPush | undefined, g: string, member: string): string[] {
  return (
    info?.agents
      .flatMap((agent) => agent.groups)
      .find((entry) => entry.g === g && entry.member === member)?.watchers ?? []
  );
}

export function requestWatch(group: Group, ai: AIMemberRow) {
  return group.home.command({
    t: 'post',
    g: group.g,
    kind: 'confirm',
    thread: null,
    main: true,
    text: '',
    mentions: [],
    re: null,
    body: { what: 'watch', ai: ai.id },
  });
}

export function stopWatch(host: Home, g: string, member: string, user: string) {
  return host.command({ t: 'watch.end', to: host.device, g, member, user });
}

export class Watch {
  phase = $state<'idle' | 'starting' | 'active' | 'relay' | 'ended' | 'error'>('idle');
  errorCode = $state('');
  #run = 0;
  #channel: PtyChannel | undefined;

  constructor(
    readonly g: string,
    readonly member: string,
    readonly write: Write,
  ) {}

  follow(target: Target | undefined, path: Path | undefined) {
    if (!target || !path) return;
    if (path.kind === 'relay') {
      this.handBack();
      this.phase = 'relay';
    } else if (this.phase === 'idle') void this.#start(target);
  }

  handBack() {
    this.#run += 1;
    const channel = this.#channel;
    this.#channel = undefined;
    if (channel) {
      channel.onmessage = null;
      channel.onclose = null;
      channel.close();
    }
    this.phase = 'ended';
  }

  async #start({ host }: Target) {
    const run = ++this.#run;
    this.phase = 'starting';
    const answer = await host
      .ctl!.request({ t: 'watch.start', g: this.g, member: this.member })
      .catch(lost);
    if (run !== this.#run) return;
    if (answer.err) {
      this.errorCode = answer.err.code;
      this.phase = 'error';
      return;
    }
    this.#open(host);
  }

  #open(host: Home) {
    const channel = host.ctl?.channel?.(`watch:${this.g}:${this.member}`);
    if (!channel) {
      this.errorCode = 'lost';
      this.phase = 'error';
      return;
    }
    this.#channel = channel;
    channel.binaryType = 'arraybuffer';
    channel.onmessage = ({ data }: { data: unknown }) => {
      this.write(toTerminalChunk(data)!);
    };
    channel.onclose = () => this.handBack();
    this.phase = 'active';
  }
}
