import './styles/tokens.css';
import './styles/base.css';
import { boot } from './lib/boot';

boot(document.body, {
  crypto,
  css: CSS,
  idb: () => indexedDB,
});
