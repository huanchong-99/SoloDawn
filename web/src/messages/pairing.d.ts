// The keys of the area `pairing`, for the type checker (texts.ts).
import type texts from './pairing.en.json';

declare module './texts' {
  interface Areas {
    pairing: typeof texts;
  }
}
