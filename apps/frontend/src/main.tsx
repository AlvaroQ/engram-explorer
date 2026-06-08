import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App.tsx';
import './lib/i18n.ts';
import './styles.css';

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('#root element missing in index.html');
}

createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
