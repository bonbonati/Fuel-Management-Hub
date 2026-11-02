/**
 * Pure formatting helpers (no React). Amounts are SDG integers; the apps
 * always show Latin digits for money (as in the prototype) and the currency as
 * "ج.س" (Arabic) or "SDG" (English).
 */
import type { Lang, Plate } from './api/types';
import { translators } from './i18n';

/** 18450000 → "18,450,000" (rounded). */
export const sar = (n: number): string => Math.round(n).toLocaleString('en-US');

/** Currency label. */
export const cur = (lang: Lang): string => (lang === 'ar' ? 'ج.س' : 'SDG');

/** "200,000 ج.س" / "200,000 SDG". */
export const curTxt = (n: number, lang: Lang): string => `${sar(n)} ${cur(lang)}`;

/** Compact amount: ≥1M → "1.35M"/"1.35 مليون" (one decimal from 10M), else full. */
export function short(n: number, lang: Lang): string {
  const a = Math.abs(n);
  if (a < 1e6) return sar(n);
  return (n / 1e6).toFixed(a >= 1e7 ? 1 : 2) + (lang === 'ar' ? ' مليون' : 'M');
}

/** "used / limit", in millions when the limit is ≥1M. */
export function pair(a: number, b: number, lang: Lang): string {
  return b >= 1e6 ? `${(a / 1e6).toFixed(2)} / ${(b / 1e6).toFixed(2)} ${lang === 'ar' ? 'مليون' : 'M'}` : `${sar(a)} / ${sar(b)}`;
}

/** "+300,000" / "زيادة 300,000". */
export const plus = (n: number, lang: Lang): string => (lang === 'ar' ? `زيادة ${short(n, lang)}` : `+${short(n, lang)}`);

/** Latin → Arabic-Indic digits ("4410" → "٤٤١٠"), as printed on plates. */
export const arDig = (s: string | number): string => String(s).replace(/\d/g, (d) => '٠١٢٣٤٥٦٧٨٩'[Number(d)]);

/** Plate as text in the current language: "٧٧٨١ خ ر ط" / "7781 KRT". */
export const plateTxt = (p: Plate, lang: Lang): string => (lang === 'ar' ? `${arDig(p.digits)} ${p.arabic}` : `${p.digits} ${p.latin}`);

/** Litres for an amount at a price per litre. */
export const litres = (amount: number, pricePerLitre: number): number => (pricePerLitre > 0 ? amount / pricePerLitre : 0);

/** HH:MM in local time. */
export const hm = (d: Date): string => d.toTimeString().slice(0, 5);

const sameDay = (a: Date, b: Date) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

/**
 * Transaction/request time label like the prototype: "Now 10:42" (< 1 min),
 * "Today 07:42", "Yesterday 18:10", otherwise "20 Oct".
 */
export function whenLabel(iso: string | null | undefined, lang: Lang, now: Date = new Date()): string {
  if (!iso) return '';
  const { T } = translators(lang);
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  if (now.getTime() - d.getTime() < 60_000 && now.getTime() >= d.getTime() - 60_000) return T(`الآن ${hm(d)}`, `Now ${hm(d)}`);
  if (sameDay(d, now)) return T(`اليوم ${hm(d)}`, `Today ${hm(d)}`);
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (sameDay(d, y)) return T(`أمس ${hm(d)}`, `Yesterday ${hm(d)}`);
  return dateLabel(iso, lang);
}

/** "20 أكتوبر" / "20 Oct" (Latin digits in both languages); year added when not the current one. */
export function dateLabel(iso: string, lang: Lang, now: Date = new Date()): string {
  const d = new Date(iso.length === 10 ? iso + 'T12:00:00' : iso);
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: lang === 'ar' ? 'long' : 'short' };
  if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
  return d.toLocaleDateString(lang === 'ar' ? 'ar-u-nu-latn' : 'en-GB', opts);
}

/** Relative age for the security log: "Just now", "14 min ago", "2 hours ago", else a date. */
export function agoLabel(iso: string, lang: Lang, now: Date = new Date()): string {
  const { T } = translators(lang);
  const mins = Math.floor((now.getTime() - new Date(iso).getTime()) / 60_000);
  if (mins < 1) return T('الآن', 'Just now');
  if (mins < 60) return T(`قبل ${mins} دقيقة`, `${mins} min ago`);
  const h = Math.floor(mins / 60);
  if (h < 24) return h === 1 ? T('قبل ساعة', '1 hour ago') : T(`قبل ${h} ساعات`, `${h} hours ago`);
  return dateLabel(iso, lang, now);
}

/** Month name for a "YYYY-MM" string: "نوفمبر" / "November". */
export function monthName(ym: string, lang: Lang): string {
  const d = new Date(ym + '-15T12:00:00');
  return d.toLocaleDateString(lang === 'ar' ? 'ar' : 'en-GB', { month: 'long' });
}

/** Initials for an avatar: "Abdurahman Hassan" → "AH", Arabic letters spaced. */
export function initials(name: string, lang: Lang): string {
  return name
    .split(' ')
    .filter(Boolean)
    .map((w) => w[0])
    .slice(0, 2)
    .join(lang === 'ar' ? ' ' : '');
}
