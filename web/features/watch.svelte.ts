// For tests of what is read reactively outside a component.

/** Runs `read` now and again whenever something it read changes. The function returned ends it. */
export function watch(read: () => void) {
  return $effect.root(() => {
    $effect(read);
  });
}
