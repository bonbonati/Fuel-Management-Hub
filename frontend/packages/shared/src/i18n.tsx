/**
 * Bilingual UI support. Arabic (RTL) is the default; English is the second
 * language. Mirrors the prototype's helpers:
 *   T(ar, en)  pick the inline string for the current language
 *   N(text)    read a bilingual {ar, en} value from the API
 * The chosen language is the only preference persisted between visits
 * (localStorage "petro-lang").
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import type { Lang, Text } from './api/types';

export const LANG_KEY = 'petro-lang';

export type TFn = (ar: string, en: string) => string;
export type NFn = (t: Text | string | null | undefined) => string;

/** Builds the T and N helpers for a language (pure; used by tests and non-React code). */
export function translators(lang: Lang): { T: TFn; N: NFn } {
  return {
    T: (ar, en) => (lang === 'ar' ? ar : en),
    N: (t) => (t == null ? '' : typeof t === 'string' ? t : t[lang]),
  };
}

function initialLang(): Lang {
  try {
    const l = localStorage.getItem(LANG_KEY);
    if (l === 'ar' || l === 'en') return l;
  } catch {
    /* storage unavailable (private mode): fall back to Arabic */
  }
  return 'ar';
}

interface I18n {
  lang: Lang;
  T: TFn;
  N: NFn;
  setLang: (l: Lang) => void;
  toggleLang: () => void;
}

const Ctx = createContext<I18n | null>(null);

/** Provides the language and keeps <html lang/dir> in sync (RTL for Arabic). */
export function I18nProvider({ children, initial }: { children: ReactNode; initial?: Lang }) {
  const [lang, setLangState] = useState<Lang>(initial ?? initialLang);
  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(LANG_KEY, l);
    } catch {
      /* ignore */
    }
  }, []);
  useEffect(() => {
    document.documentElement.lang = lang;
    document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
  }, [lang]);
  const value = useMemo<I18n>(
    () => ({ lang, ...translators(lang), setLang, toggleLang: () => setLang(lang === 'ar' ? 'en' : 'ar') }),
    [lang, setLang],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

/** Returns the current language and the T/N helpers. */
export function useI18n(): I18n {
  const v = useContext(Ctx);
  if (!v) throw new Error('useI18n must be used inside <I18nProvider>');
  return v;
}

/** Shorthand for bilingual literals in data (prototype W). */
export const W = (ar: string, en: string): Text => ({ ar, en });
