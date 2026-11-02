import { describe, expect, it } from 'vitest';
import { agoLabel, arDig, curTxt, dateLabel, initials, litres, pair, plateTxt, plus, sar, short, whenLabel } from '../src/format';

describe('money formatting', () => {
  it('formats SDG with thousands separators and rounding', () => {
    expect(sar(18450000)).toBe('18,450,000');
    expect(sar(1234.6)).toBe('1,235');
    expect(curTxt(200000, 'ar')).toBe('200,000 ج.س');
    expect(curTxt(200000, 'en')).toBe('200,000 SDG');
  });

  it('shortens millions like the prototype', () => {
    expect(short(400000, 'en')).toBe('400,000');
    expect(short(1350000, 'en')).toBe('1.35M');
    expect(short(18450000, 'en')).toBe('18.4M'); // one decimal from 10M
    expect(short(1500000, 'ar')).toBe('1.50 مليون');
    expect(short(-2000000, 'en')).toBe('-2.00M');
  });

  it('renders used/limit pairs in millions only for large limits', () => {
    expect(pair(768000, 1350000, 'en')).toBe('0.77 / 1.35 M');
    expect(pair(300000, 750000, 'en')).toBe('300,000 / 750,000');
  });

  it('renders approved increases', () => {
    expect(plus(150000, 'en')).toBe('+150,000');
    expect(plus(150000, 'ar')).toBe('زيادة 150,000');
  });
});

describe('plates and digits', () => {
  it('converts to Arabic-Indic digits', () => {
    expect(arDig('4410')).toBe('٤٤١٠');
    expect(arDig(7)).toBe('٧');
  });

  it('formats plates per language', () => {
    const p = { digits: '7781', latin: 'KRT', arabic: 'خ ر ط' };
    expect(plateTxt(p, 'en')).toBe('7781 KRT');
    expect(plateTxt(p, 'ar')).toBe('٧٧٨١ خ ر ط');
  });
});

describe('litres', () => {
  it('divides by the price and guards zero', () => {
    expect(litres(200000, 3600)).toBeCloseTo(55.56, 2);
    expect(litres(100, 0)).toBe(0);
  });
});

describe('time labels', () => {
  const now = new Date(2026, 10, 2, 10, 0, 0); // 2 Nov 2026 10:00 local

  it('labels now, today, yesterday and older dates', () => {
    expect(whenLabel(new Date(2026, 10, 2, 9, 59, 30).toISOString(), 'en', now)).toBe('Now 09:59');
    expect(whenLabel(new Date(2026, 10, 2, 7, 42).toISOString(), 'en', now)).toBe('Today 07:42');
    expect(whenLabel(new Date(2026, 10, 2, 7, 42).toISOString(), 'ar', now)).toBe('اليوم 07:42');
    expect(whenLabel(new Date(2026, 10, 1, 18, 10).toISOString(), 'en', now)).toBe('Yesterday 18:10');
    expect(whenLabel(new Date(2026, 9, 20, 9, 0).toISOString(), 'en', now)).toBe('20 Oct');
    expect(whenLabel(null, 'en', now)).toBe('');
  });

  it('uses Latin digits for Arabic dates', () => {
    expect(dateLabel('2026-10-20', 'ar', now)).toMatch(/^20 أكتوبر$/);
  });

  it('labels security log ages', () => {
    const ago = (m: number) => new Date(now.getTime() - m * 60_000).toISOString();
    expect(agoLabel(ago(0), 'en', now)).toBe('Just now');
    expect(agoLabel(ago(14), 'en', now)).toBe('14 min ago');
    expect(agoLabel(ago(14), 'ar', now)).toBe('قبل 14 دقيقة');
    expect(agoLabel(ago(60), 'en', now)).toBe('1 hour ago');
    expect(agoLabel(ago(180), 'en', now)).toBe('3 hours ago');
  });
});

describe('initials', () => {
  it('takes two initials, spaced in Arabic', () => {
    expect(initials('Abdurahman Hassan', 'en')).toBe('AH');
    expect(initials('عبدالرحمن حسن', 'ar')).toBe('ع ح');
    expect(initials('Single', 'en')).toBe('S');
  });
});
