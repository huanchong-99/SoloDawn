import './styles/tokens.css';
import './styles/base.css';
import { boot } from './lib/boot';

const start = () =>
  boot(document.body, {
    crypto,
    css: CSS,
    idb: () => indexedDB,
  });

// A development server plays a whole world behind the connection at `?demo`: computers, people
// and AIs (web/features/sim). The build has none of it.
if (import.meta.env.DEV && new URLSearchParams(location.search).has('demo'))
  void import('../features/sim/install').then(({ install }) => install()).then(start);
else start();
