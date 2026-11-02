import { act, render, renderHook, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../src/api/client';
import { I18nProvider, LANG_KEY, useI18n } from '../src/i18n';
import { AppProviders } from '../src/providers';
import { useAction } from '../src/queries';
import { loadSession, useSession } from '../src/session';
import { FuelTag, PlateTag, Quota, ReqPill, StatusPill } from '../src/ui/atoms';

const wrap = ({ children }: { children: ReactNode }) => <AppProviders storageKey="test-session">{children}</AppProviders>;

afterEach(() => vi.unstubAllGlobals());

describe('I18nProvider', () => {
  it('defaults to Arabic RTL and persists the toggle', async () => {
    function Probe() {
      const { T, lang, toggleLang } = useI18n();
      return <button onClick={toggleLang}>{T('عربي', 'English')}:{lang}</button>;
    }
    render(<I18nProvider><Probe /></I18nProvider>);
    expect(screen.getByRole('button')).toHaveTextContent('عربي:ar');
    expect(document.documentElement.dir).toBe('rtl');
    await userEvent.click(screen.getByRole('button'));
    expect(screen.getByRole('button')).toHaveTextContent('English:en');
    expect(document.documentElement.dir).toBe('ltr');
    expect(localStorage.getItem(LANG_KEY)).toBe('en');
  });

  it('restores the saved language', () => {
    localStorage.setItem(LANG_KEY, 'en');
    const { result } = renderHook(() => useI18n(), { wrapper: ({ children }) => <I18nProvider>{children}</I18nProvider> });
    expect(result.current.lang).toBe('en');
    expect(result.current.N({ ar: 'أ', en: 'A' })).toBe('A');
    expect(result.current.N(null)).toBe('');
  });
});

describe('session', () => {
  const user = { id: 'u1', username: 'D-3071', role: 'driver', name: { ar: 'ع', en: 'A' }, active: true, demo: true };

  it('discards expired or malformed stored sessions', () => {
    localStorage.setItem('k', JSON.stringify({ token: 't', user, expiresAt: '2000-01-01T00:00:00Z' }));
    expect(loadSession('k')).toBeNull();
    localStorage.setItem('k', '{oops');
    expect(loadSession('k')).toBeNull();
    localStorage.setItem('k', JSON.stringify({ token: 't', user, expiresAt: '2999-01-01T00:00:00Z' }));
    expect(loadSession('k')?.user.username).toBe('D-3071');
  });

  it('logs in, persists, sends the token, and logs out', async () => {
    const fetch = vi.fn(async (url: string) => {
      if (url.endsWith('/login')) return new Response(JSON.stringify({ accessToken: 'tok', tokenType: 'Bearer', expiresAt: '2999-01-01T00:00:00Z', user }), { status: 200 });
      return new Response('{}', { status: 200 });
    });
    vi.stubGlobal('fetch', fetch);
    const { result } = renderHook(() => useSession(), { wrapper: wrap });

    await act(async () => { await result.current.login('D-3071', 'demo1234'); });
    expect(result.current.user?.role).toBe('driver');
    expect(loadSession('test-session')?.token).toBe('tok');

    act(() => result.current.logout());
    expect(result.current.user).toBeNull();
    expect(localStorage.getItem('test-session')).toBeNull();
  });
});

describe('useAction', () => {
  it('toasts success and returns the result', async () => {
    const { result } = renderHook(() => useAction(), { wrapper: wrap });
    let r: number | undefined;
    await act(async () => { r = await result.current(async () => 42, 'Saved'); });
    expect(r).toBe(42);
    expect(screen.getByRole('status')).toHaveTextContent('Saved');
  });

  it('toasts the server message in the current language and swallows the error', async () => {
    localStorage.setItem(LANG_KEY, 'ar');
    const { result } = renderHook(() => useAction(), { wrapper: wrap });
    let r: unknown = 'unset';
    await act(async () => {
      r = await result.current(async () => { throw new ApiError(409, 'limit_exceeded', { ar: 'تجاوز الحد', en: 'Over limit' }); }, 'nope');
    });
    expect(r).toBeUndefined();
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('تجاوز الحد'));
  });
});

describe('atoms', () => {
  const inEnglish = (ui: ReactNode) => render(<I18nProvider initial="en">{ui}</I18nProvider>);

  it('renders plates with both scripts', () => {
    inEnglish(<PlateTag p={{ digits: '9087', latin: 'BH', arabic: 'ب ح' }} />);
    expect(screen.getByText('9087 BH')).toBeInTheDocument();
    expect(screen.getByText('٩٠٨٧ ب ح')).toBeInTheDocument();
  });

  it('renders fuel tags with grade classes', () => {
    const { container } = inEnglish(<FuelTag f="Diesel" />);
    expect(container.querySelector('.fgdz')).toHaveTextContent('Diesel');
  });

  it('caps the quota bar at 100% and colors it', () => {
    const { container } = inEnglish(<Quota used={2000} lim={1000} />);
    const bar = container.querySelector('.quota i') as HTMLElement;
    expect(bar.style.width).toBe('100%');
    expect(bar.className).toBe('rd');
  });

  it('labels statuses', () => {
    inEnglish(<><StatusPill t={{ status: 'auth' }} /><StatusPill t={{ status: 'fueling' }} /><ReqPill r={{ status: 'rejected' }} /></>);
    expect(screen.getByText('Authorized')).toBeInTheDocument();
    expect(screen.getByText('Dispensing')).toBeInTheDocument();
    expect(screen.getByText('Declined')).toBeInTheDocument();
  });
});
