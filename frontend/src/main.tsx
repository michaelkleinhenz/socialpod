import React, { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

// Expose React globally for libraries that use the legacy JSX transform
// (e.g. react-filerobot-image-editor / styled-components).
(window as any).React = React;
import App from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
