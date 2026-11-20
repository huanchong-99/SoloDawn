type ActiveSession = (signal: AbortSignal) => Promise<void>;

// Keep a session error (including its own AbortError) distinct from a stolen Web Lock.
async function runSession(active: ActiveSession, signal: AbortSignal) {
  try {
    await active(signal);
  } catch (error) {
    return { error };
  }
}

// True on normal session end; false when another tab holds the lock.
async function trySession(name: string, options: LockOptions, active: ActiveSession) {
  const session = new AbortController();
  const result = await navigator.locks
    .request(name, options, (lock) => lock && runSession(active, session.signal))
    .catch((error: unknown) => {
      if (!(error instanceof DOMException && error.name === 'AbortError')) throw error;
      session.abort();
      return null;
    });
  if (result) throw result.error;
  return result !== null;
}

/** Only active may start signalling, peers, reconnects or probes. Its promise holds the lock
 *  until normal session end; on abort it must cancel that work and close its connections.
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
