// happy-dom sets navigator.locks to null; every browser in the baseline implements Web Locks
// (planning/flows/connect.md section F), so unit and feature suites get a granted-lock stub unless
// a test installs its own LockManager.
if (!navigator.locks) {
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (_name: string, _options: unknown, callback: (lock: object) => unknown) =>
        Promise.resolve(callback({})),
    },
  });
}
