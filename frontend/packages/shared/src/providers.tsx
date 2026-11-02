/**
 * The provider stack both apps mount at their root:
 * React Query → session (auth) → language → toasts.
 */
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';
import { I18nProvider } from './i18n';
import { createQueryClient } from './queries';
import { SessionProvider } from './session';
import { ToastProvider } from './toast';

export function AppProviders({ storageKey, baseUrl, client, children }: { storageKey: string; baseUrl?: string; client?: QueryClient; children: ReactNode }) {
  const [qc] = useState(() => client ?? createQueryClient());
  return (
    <QueryClientProvider client={qc}>
      <SessionProvider storageKey={storageKey} baseUrl={baseUrl}>
        <I18nProvider>
          <ToastProvider>{children}</ToastProvider>
        </I18nProvider>
      </SessionProvider>
    </QueryClientProvider>
  );
}
