/**
 * Signed-in session: the access token and the user it belongs to.
 *
 * The session is kept in localStorage under an app-specific key, so the web
 * admin and the mobile app (same origin in the local environment) can be
 * signed in as different people at the same time — e.g. the fleet manager on
 * the web and a driver on the phone, which is how the demo is shown.
 */
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api, configureApi } from './api/client';
import type { TokenResponse, User } from './api/types';

export interface Session {
  token: string;
  user: User;
  expiresAt: string;
}

/** Reads a stored session, discarding malformed or expired ones. */
export function loadSession(key: string, now: number = Date.now()): Session | null {
  try {
    const s = JSON.parse(localStorage.getItem(key) ?? 'null') as Session | null;
    if (!s?.token || !s.user || !s.expiresAt || Date.parse(s.expiresAt) <= now) return null;
    return s;
  } catch {
    return null;
  }
}

function storeSession(key: string, s: Session | null) {
  try {
    if (s) localStorage.setItem(key, JSON.stringify(s));
    else localStorage.removeItem(key);
  } catch {
    /* storage unavailable: session lives in memory only */
  }
}

interface SessionApi {
  session: Session | null;
  user: User | null;
  login: (username: string, password: string) => Promise<User>;
  demoLogin: (username: string) => Promise<User>;
  logout: () => void;
}

const Ctx = createContext<SessionApi | null>(null);

/**
 * Provides the session and configures the shared API client to send its
 * token. Must be rendered inside a QueryClientProvider: switching users clears
 * all cached data.
 */
export function SessionProvider({ storageKey, baseUrl = '', children }: { storageKey: string; baseUrl?: string; children: ReactNode }) {
  const qc = useQueryClient();
  const [session, setSession] = useState<Session | null>(() => loadSession(storageKey));
  const ref = useRef(session);
  ref.current = session;

  const apply = useCallback(
    (s: Session | null) => {
      storeSession(storageKey, s);
      ref.current = s;
      qc.clear();
      setSession(s);
    },
    [qc, storageKey],
  );
  const applyRef = useRef(apply);
  applyRef.current = apply;

  // Configure the client once; it reads the current token through the ref.
  useState(() =>
    configureApi({ baseUrl, getToken: () => ref.current?.token ?? null, onUnauthorized: () => applyRef.current(null) }),
  );

  const fromToken = useCallback(
    (r: TokenResponse) => {
      apply({ token: r.accessToken, user: r.user, expiresAt: r.expiresAt });
      return r.user;
    },
    [apply],
  );

  const value = useMemo<SessionApi>(
    () => ({
      session,
      user: session?.user ?? null,
      login: async (u, p) => fromToken(await api.auth.login(u, p)),
      demoLogin: async (u) => fromToken(await api.auth.demoLogin(u)),
      logout: () => apply(null),
    }),
    [session, fromToken, apply],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

/** Returns the session API. */
export function useSession(): SessionApi {
  const v = useContext(Ctx);
  if (!v) throw new Error('useSession must be used inside <SessionProvider>');
  return v;
}
