import '@petro/shared/styles.css';
import { AppProviders } from '@petro/shared';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';

// The web admin is served by the gateway, so the API is same-origin (baseUrl '').
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppProviders storageKey="petro-web-session">
      <App />
    </AppProviders>
  </StrictMode>,
);
