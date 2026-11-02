/**
 * Client-side domain helpers ported from the prototype: labels for fuel
 * grades and request types, VAT, budget/usage aggregation and the fleet
 * alert list. Authoritative rules (limits, caps, authorization) live in the
 * backend services; these helpers only derive what screens display.
 */
import type { Driver, DriverRequest, Fuel, Prices, Staff, Tax, Text, Tier, Transaction, Vehicle } from './api/types';
import { curTxt, plateTxt, short } from './format';
import type { NFn, TFn } from './i18n';
import { W } from './i18n';
import type { Lang } from './api/types';

/** Fallback prices (SDG/L) until /stations/fuel-prices has loaded. */
export const DEFAULT_PRICES: Prices = { '91': 3420, '95': 3600, Diesel: 3250 };

export const FUEL: Record<Fuel, Text> = { '91': W('بنزين', 'Benzene'), '95': W('سوبر', 'Super'), Diesel: W('جازولين', 'Diesel') };
export const FUELFULL: Record<Fuel, Text> = {
  '91': W('بنزين عادي', 'Regular benzene'),
  '95': W('بنزين سوبر', 'Super benzene'),
  Diesel: W('جازولين (ديزل)', 'Diesel (gasoil)'),
};
/** CSS class of a fuel grade tag. */
export const fuelClass = (f: Fuel): string => (f === '91' ? 'fg91' : f === '95' ? 'fg95' : 'fgdz');

export const REQTYPE: Record<DriverRequest['type'], Text> = {
  limit: W('زيادة الحد الشهري', 'Monthly limit increase'),
  activate: W('إعادة التفعيل', 'Reactivation'),
};
export const REASON: Record<DriverRequest['reason'], Text> = {
  trip: W('رحلة خارج المدينة', 'Out-of-town trip'),
  extra: W('مهام عمل إضافية', 'Additional assignments'),
  emergency: W('حالة طارئة', 'Emergency'),
  review: W('مراجعة قرار الإيقاف', 'Review of suspension'),
  other: W('سبب آخر', 'Other'),
};
/** Reasons offered per request type in the driver's request sheet. */
export const REASONS_FOR: Record<DriverRequest['type'], DriverRequest['reason'][]> = {
  limit: ['trip', 'extra', 'emergency', 'other'],
  activate: ['review', 'emergency', 'other'],
};

export const roleName = (r: Staff['role']): Text => (r === 'sup' ? W('مشرف المحطة', 'Station supervisor') : W('عامل مضخة', 'Pump attendant'));
export const shiftName = (s: string): string => ({ A: 'A · 06:00–14:00', B: 'B · 14:00–22:00', C: 'C · 22:00–06:00' })[s] ?? s;

/** VAT contained in a VAT-inclusive amount (0 when VAT is off). */
export const vatOf = (amount: number, tax: Tax | undefined): number =>
  tax?.enabled ? amount - amount / (1 + tax.rate / 100) : 0;

/** Quota bar color class: amber from 85%, red from 98%. */
export const quotaCls = (pct: number): string => (pct >= 98 ? 'rd' : pct >= 85 ? 'am' : '');

/** Usage of a vehicle this month (0 when unknown). */
export const usedOf = (v: Vehicle, usage: Record<string, number> | undefined): number => usage?.[v.id] ?? 0;

export interface TierStats {
  vs: Vehicle[];
  budget: number;
  spent: number;
  remain: number;
  pct: number;
  extra: number;
}

/** Budget/spend of a tier: sum over its vehicles' limits and usage. */
export function tierStats(t: Tier, vehicles: Vehicle[], usage: Record<string, number> | undefined): TierStats {
  const vs = vehicles.filter((v) => v.tierId === t.id);
  const budget = vs.reduce((a, v) => a + v.limit, 0);
  const spent = vs.reduce((a, v) => a + usedOf(v, usage), 0);
  return { vs, budget, spent, remain: Math.max(0, budget - spent), pct: (spent / Math.max(1, budget)) * 100, extra: vs.reduce((a, v) => a + v.extra, 0) };
}

/** Fleet-wide limit, spend and litres this month. */
export function fleetTotals(vehicles: Vehicle[], usage: Record<string, number> | undefined, prices: Prices) {
  const lim = vehicles.reduce((a, v) => a + v.limit, 0);
  const used = vehicles.reduce((a, v) => a + usedOf(v, usage), 0);
  const litres = vehicles.reduce((a, v) => a + usedOf(v, usage) / (prices[v.fuel] || 1), 0);
  return { lim, used, pct: (used / Math.max(1, lim)) * 100, litres };
}

/** Illustrative weekly spend chart (first three weeks are demo history, as in the prototype). */
export const weeklySpend = (used: number): number[] => [1230000, 1590000, 1470000, Math.max(0, Math.round(used - 4290000))];

/** What clicking an alert's action does. */
export type AlertAction =
  | { kind: 'review-requests' }
  | { kind: 'reactivate-driver'; id: string }
  | { kind: 'reactivate-vehicle'; id: string }
  | { kind: 'raise-limit'; id: string; amount: number }
  | { kind: 'order-tag'; id: string };

export interface Alert {
  color: 'bl' | 'rd' | 'am';
  icon: string;
  tag: string;
  title: string;
  detail: string;
  actionLabel: string;
  action: AlertAction;
}

/** Amount added by the "Add SDG 300,000" alert action. */
export const RAISE_AMOUNT = 300_000;

/**
 * Builds the fleet manager's alert list (prototype fleetAlerts): pending
 * requests, suspended drivers, suspended vehicles, vehicles at/near their
 * limit (≥98% / ≥85%) and low RFID tag batteries (<15%).
 */
export function fleetAlerts(
  d: { requests: DriverRequest[]; drivers: Driver[]; vehicles: Vehicle[]; usage: Record<string, number> | undefined },
  lang: Lang,
  T: TFn,
  N: NFn,
): Alert[] {
  const out: Alert[] = [];
  const driverName = (v: Vehicle) => {
    const dr = v.driverId ? d.drivers.find((x) => x.id === v.driverId) : undefined;
    return dr ? N(dr.name) : T('غير مخصص', 'Unassigned');
  };
  for (const r of d.requests.filter((x) => x.status === 'pending')) {
    const dr = d.drivers.find((x) => x.id === r.driverId);
    out.push({
      color: 'bl', icon: 'inbox', tag: T('طلب جديد', 'New request'), title: dr ? N(dr.name) : '',
      detail: r.type === 'limit'
        ? T(`يطلب زيادة الحد الشهري بمبلغ ${curTxt(r.amount, lang)}.`, `Requests a monthly limit increase of ${curTxt(r.amount, lang)}.`)
        : T('يطلب إعادة تفعيل الحساب أو المركبة.', 'Requests reactivation of the account or vehicle.'),
      actionLabel: T('مراجعة الطلب', 'Review request'), action: { kind: 'review-requests' },
    });
  }
  for (const dr of d.drivers.filter((x) => !x.active)) {
    out.push({
      color: 'rd', icon: 'user', tag: T('سائق موقوف', 'Driver suspended'), title: N(dr.name),
      detail: T('لا يمكن لهذا السائق استخدام تصريح الوقود حتى يُعاد تفعيله.', 'This driver cannot use the fuel pass until reactivated.'),
      actionLabel: T('إعادة التفعيل', 'Reactivate'), action: { kind: 'reactivate-driver', id: dr.id },
    });
  }
  for (const v of d.vehicles) {
    const used = usedOf(v, d.usage);
    const p = used / Math.max(1, v.limit);
    const title = `${N(v.name)} · ${plateTxt(v.plate, lang)}`;
    const nm = driverName(v);
    const raise = T(`إضافة ${short(RAISE_AMOUNT, lang)} ج.س`, `Add SDG ${short(RAISE_AMOUNT, lang)}`);
    if (!v.active) {
      out.push({
        color: 'rd', icon: 'lock', tag: T('مركبة موقوفة', 'Vehicle suspended'), title,
        detail: T(`لا يمكن لـ ${nm} التعبئة بهذه المركبة حتى يُعاد تفعيلها.`, `${nm} cannot fuel this vehicle until it is reactivated.`),
        actionLabel: T('إعادة التفعيل', 'Reactivate'), action: { kind: 'reactivate-vehicle', id: v.id },
      });
    } else if (p >= 0.98) {
      out.push({
        color: 'rd', icon: 'gauge', tag: T('استُنفد الحد', 'Limit reached'), title,
        detail: T(`استُنفد الحد الشهري لـ ${nm}.`, `${nm} has used the full monthly limit.`),
        actionLabel: raise, action: { kind: 'raise-limit', id: v.id, amount: RAISE_AMOUNT },
      });
    } else if (p >= 0.85) {
      const left = curTxt(v.limit - used, lang);
      out.push({
        color: 'am', icon: 'gauge', tag: T('قرب الحد', 'Near limit'), title,
        detail: T(`المتبقي لـ ${nm} هذا الشهر ${left}.`, `${nm} has ${left} left this month.`),
        actionLabel: raise, action: { kind: 'raise-limit', id: v.id, amount: RAISE_AMOUNT },
      });
    }
    if (v.tagBattery < 15) {
      out.push({
        color: 'am', icon: 'battery', tag: T('بطارية الشريحة منخفضة', 'Low tag battery'), title,
        detail: T('انخفضت إشارة شريحة الوقود إلى أقل من 15%.', 'The fuel tag signal is below 15%.'),
        actionLabel: T('طلب شريحة بديلة', 'Order a replacement tag'), action: { kind: 'order-tag', id: v.id },
      });
    }
  }
  return out;
}

/** Pump state at a station: offline, busy (open transaction) or free. */
export function pumpState(online: boolean, number: number, stationTx: Transaction[]): 'off' | 'busy' | 'free' {
  if (!online) return 'off';
  return stationTx.some((t) => t.pump === number && t.status !== 'done') ? 'busy' : 'free';
}
