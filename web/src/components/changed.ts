/** Marks a card from the first time its state changes after it was drawn (`data-changed`), so
 *  that the styles let the new state land (base.css, 5) while what a chat opens with is simply
 *  there. An attribute, because a component rewrites its node's classes. */
export const changed = (node: HTMLElement, _state: unknown) => ({
  update: () => void (node.dataset.changed = ''),
});
