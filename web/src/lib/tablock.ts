type ActiveSession = (signal: AbortSignal) => Promise<void>;

/** How a session ended: on its own, or with the error it rejected with. */
type Ended = { ok: true } | { ok: false; error: unknown };

// True when the session ran to its end; false when another tab held the lock or took it. Only a
// rejection of request() itself is a steal: an error of the session, an AbortError included, is
// carried out of the lock's callback and thrown once request() has resolved.
async function trySession(name: string, options: LockOptions, active: ActiveSession) {
  const session = new AbortController();
  let ended: Ended | null;
  try {
    ended = await navigator.locks.request(
      name,
      options,
      (lock) =>
        lock &&
        active(session.signal).then(
          (): Ended => ({ ok: true }),
          (error: unknown): Ended => ({ ok: false, error }),
        ),
    );
  } catch (error) {
    if ((error as Error).name !== 'AbortError') throw error;
    session.abort();
    return false;
  }
  if (ended === null) return false;
  if (!ended.ok) throw ended.error;
  return true;
}

/** Only active may start signalling, peers, reconnects or probes. Its promise holds the lock
 *  until the session ends; on abort it must cancel that work and close its connections.
 *  waitForUseHere reports the other-tab state and resolves only on the person's choice. */
export async function runTab(
  device: string,
  active: ActiveSession,
  waitForUseHere: () => Promise<void>,
): Promise<void> {
  const name = `solodawn-device-${device}`;
  let options: LockOptions = { ifAvailable: true };
  while (!(await trySession(name, options, active))) {
    await waitForUseHere();
    options = { steal: true };
  }
}
