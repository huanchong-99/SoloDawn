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

// happy-dom eagerly creates `Animation.finished` and rejects it on `cancel()`, which Svelte's
// `out:` transition calls during normal handoff and unmount.
if (typeof Element !== 'undefined' && Element.prototype.animate) {
  const rawAnimate = Element.prototype.animate;
  Element.prototype.animate = function (...args: Parameters<Element['animate']>) {
    const animation = rawAnimate.apply(this, args);
    animation.finished.catch(() => undefined);
    return animation;
  };
}
