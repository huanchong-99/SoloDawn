type ActiveSession = (signal: AbortSignal) => Promise<void>;

// True when the session ran to its end; false when another tab held the lock or took it.
async function trySession(name: string, options: LockOptions, active: ActiveSession) {
  const session = new AbortController();
  try {
    const ended = await navigator.locks.request(
      name,
      options,
      (lock) => lock && active(session.signal).then(() => true),
    );
    return ended === true;
  } catch (error) {
    if ((error as Error).name !== 'AbortError') throw error;
    session.abort();
    return false;
  }
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
