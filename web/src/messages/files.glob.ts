// Every file of an area's texts, by path, for texts.ts: two dots in the name, as `groups.en.json`.
// A module that only holds an `import.meta.glob` is named `*.glob.ts` and is not mutated; it
// holds no logic (Stryker would wrap the pattern, which Vite must read as written).
export const files = import.meta.glob<Record<string, unknown>>('./*.*.json', {
  import: 'default',
  eager: true,
});
