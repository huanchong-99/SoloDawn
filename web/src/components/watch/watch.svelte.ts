import { confirmStatus, isWatch, type Confirm } from '../../lib/confirm';
import { lost, type Group, type Home } from '../../lib/groups.svelte';
import type { Path } from '../../lib/net.svelte';
import { Retry } from '../../lib/peer';
import { t } from '../../lib/prefs.svelte';
import type { AIMemberRow, HostInfoPush, PtyChannel } from '../../lib/proto';
import { toTerminalChunk, type BarView, type Target, type Write } from '../../lib/terminal.svelte';

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

/** This reader's request to watch the AI's terminal: their last one, unless it was settled before
 *  the AI's present state began. Consent is for one takeover
 *  (contracts/step-2/watch-team.md section 1). */
export function watchRequest(group: Group, ai: AIMemberRow): Confirm | undefined {
  const now = ({ state }: Confirm) => state.status === 'pending' || state.at >= ai.since;
  return group.main
    .filter(isWatch)
    .filter((msg) => msg.by === group.me?.id && msg.body.ai === ai.id && now(msg))
    .at(-1);
}

export function stopWatch(host: Home, g: string, member: string, user: string) {
  return host.command({ t: 'watch.end', to: host.device, g, member, user });
}

export class Watch {
  phase = $state<'idle' | 'starting' | 'active' | 'relay' | 'ended' | 'error'>('idle');
  /** Why the last start was refused, until it is answered otherwise or given up. */
  errorCode = $state('');
  /** The starts refused since the consent. */
  refusals = $state(0);
  #run = 0;
  #channel: PtyChannel | undefined;
  #consented = false;
  readonly #again = () => {
    if (this.phase === 'error') this.phase = 'idle';
  };
  #retry = new Retry(this.#again);

  constructor(
    readonly g: string,
    readonly member: string,
    readonly write: Write,
  ) {}

  follow(target: Target | undefined, path: Path | undefined) {
    if (!target || !path || this.phase === 'ended') return;
    if (path.kind === 'relay') this.#close('relay');
    else if (['idle', 'relay'].includes(this.phase)) void this.#start(target);
  }

  /** The owner confirmed. The grant reaches the computer a moment later, so a refusal is tried
   *  again from now on, after the back-off of a connection. */
  consented() {
    this.#consented = true;
    this.errorCode = '';
    this.refusals = 0;
    this.#retry.stop();
    this.#retry = new Retry(this.#again);
    this.#again();
  }

  handBack() {
    this.#close('ended');
  }

  #close(phase: 'relay' | 'ended') {
    this.#run += 1;
    this.#retry.stop();
    this.errorCode = '';
    const channel = this.#channel;
    this.#channel = undefined;
    if (channel) {
      channel.onmessage = null;
      channel.onclose = null;
      channel.close();
    }
    this.phase = phase;
  }

  async #start({ host }: Target) {
    const run = ++this.#run;
    this.phase = 'starting';
    const answer = await host
      .ctl!.request({ t: 'watch.start', g: this.g, member: this.member })
      .catch(lost);
    if (run !== this.#run) return;
    if (answer.err) this.#refused(answer.err.code);
    else this.#open(host);
  }

  #refused(code: string) {
    this.errorCode = code;
    this.refusals += 1;
    this.phase = 'error';
    if (this.#consented) this.#retry.after(0);
  }

  #open(host: Home) {
    const channel = host.ctl?.channel?.(`watch:${this.g}:${this.member}`);
    if (!channel) return this.#refused('lost');
    this.#channel = channel;
    channel.binaryType = 'arraybuffer';
    channel.onmessage = ({ data }: { data: unknown }) => {
      this.write(toTerminalChunk(data)!);
    };
    channel.onclose = () => this.handBack();
    this.errorCode = '';
    this.phase = 'active';
  }
}

const details = {
  none: 'watch.none',
  over: 'watch.over.body',
  pending: 'watch.pending',
  confirmed: 'watch.confirmed',
  denied: 'watch.again',
  expired: 'watch.again',
} as const;
type Consent = keyof typeof details;

function consentOf(ai: AIMemberRow, request: Confirm | undefined): Consent {
  if (ai.state !== 'takeover') return 'over';
  return request?.state.status ?? 'none';
}

function consentTitle(group: Group, consent: Consent, request: Confirm | undefined) {
  if (consent === 'over') return t('watch.over');
  return request ? confirmStatus(group, request) : t('watch.idle');
}

/** Whether it is the owner's consent that the watch waits for. */
function waits({ phase, errorCode }: Watch, consent: Consent) {
  if (errorCode) return errorCode === 'forbidden';
  return phase === 'idle' || (phase === 'starting' && consent !== 'none');
}

/** One refusal after the consent may be the moment before the grant, more the owner's stop. */
const asks = ({ refusals }: Watch, consent: Consent) =>
  consent === 'confirmed' ? refusals > 1 : !['over', 'pending'].includes(consent);

/** What the bar of the terminal page says to someone who watches or wants to. A start that is tried
 *  again changes nothing in it until the answer. */
export function watchBar(watch: Watch, group: Group, ai: AIMemberRow): BarView {
  const { errorCode } = watch;
  const phase = errorCode ? 'error' : watch.phase;
  const request = watchRequest(group, ai);
  const consent = consentOf(ai, request);
  if (!waits(watch, consent))
    return { title: t(`watch.${phase}`), detail: t('watch.readonly'), errorCode, handback: false };
  return {
    title: consentTitle(group, consent, request),
    detail: t(details[consent]),
    handback: false,
    ask: asks(watch, consent),
  };
}
