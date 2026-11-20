// The keys of the area `nodes`, for the type checker (texts.ts).
import type texts from './nodes.en.json';

declare module './texts' {
  interface Areas {
    nodes: typeof texts;
  }
}
