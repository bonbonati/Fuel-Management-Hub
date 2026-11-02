import { describe, expect, it } from 'vitest';
import type { Driver, DriverRequest, Tier, Transaction, Vehicle } from '../src/api/types';
import { DEFAULT_PRICES, fleetAlerts, fleetTotals, pumpState, quotaCls, tierStats, vatOf, weeklySpend } from '../src/domain';
import { translators } from '../src/i18n';

const veh = (over: Partial<Vehicle>): Vehicle => ({
  id: 'f1', orgId: 'o1', plate: { digits: '4410', latin: 'KRT', arabic: 'خ ر ط' }, name: { ar: 'إيسوزو', en: 'Isuzu' },
  driverId: 'd1', fuel: 'Diesel', tierId: 't1', customLimit: null, extra: 0, active: true, tagBattery: 90,
  baseLimit: 1_500_000, limit: 1_500_000, ...over,
});
const drv = (over: Partial<Driver>): Driver => ({
  id: 'd1', orgId: 'o1', name: { ar: 'فيصل', en: 'Faisal' }, phone: '', code: 'D-1', appActivated: true, active: true, ...over,
});
const tier: Tier = { id: 't1', orgId: 'o1', icon: 'truck', color: 'or', name: { ar: 'ش', en: 'Trucks' }, budget: 1_500_000, dailyLitres: 200, window: '24/7' };
const { T, N } = translators('en');

describe('vatOf', () => {
  it('backs VAT out of VAT-inclusive amounts', () => {
    expect(vatOf(117_000, { enabled: true, rate: 17 })).toBeCloseTo(17_000, 6);
    expect(vatOf(100_000, { enabled: true, rate: 17 })).toBeCloseTo(14_529.91, 2);
  });
  it('is zero when VAT is off or unknown', () => {
    expect(vatOf(100_000, { enabled: false, rate: 17 })).toBe(0);
    expect(vatOf(100_000, undefined)).toBe(0);
  });
});

describe('quotaCls', () => {
  it('switches color at 85% and 98%', () => {
    expect(quotaCls(84.9)).toBe('');
    expect(quotaCls(85)).toBe('am');
    expect(quotaCls(98)).toBe('rd');
  });
});

describe('tierStats and fleetTotals', () => {
  const vehicles = [veh({ id: 'f1' }), veh({ id: 'f2', limit: 1_800_000, extra: 300_000 }), veh({ id: 'f3', tierId: 't2', fuel: '95', limit: 1_200_000 })];
  const usage = { f1: 1_000_000, f2: 500_000, f3: 600_000 };

  it('aggregates the vehicles of one tier', () => {
    const s = tierStats(tier, vehicles, usage);
    expect(s.vs.map((v) => v.id)).toEqual(['f1', 'f2']);
    expect(s.budget).toBe(3_300_000);
    expect(s.spent).toBe(1_500_000);
    expect(s.remain).toBe(1_800_000);
    expect(s.extra).toBe(300_000);
    expect(s.pct).toBeCloseTo(45.45, 2);
  });

  it('never reports negative remaining budget', () => {
    expect(tierStats(tier, [veh({ limit: 100 })], { f1: 500 }).remain).toBe(0);
  });

  it('totals the fleet including litres at current prices', () => {
    const t = fleetTotals(vehicles, usage, DEFAULT_PRICES);
    expect(t.lim).toBe(4_500_000);
    expect(t.used).toBe(2_100_000);
    expect(t.litres).toBeCloseTo(1_500_000 / 3250 + 600_000 / 3600, 6);
  });

  it('treats missing usage as zero', () => {
    expect(fleetTotals(vehicles, undefined, DEFAULT_PRICES).used).toBe(0);
  });

  it('derives the last weekly bar from the month spend', () => {
    expect(weeklySpend(5_000_000)[3]).toBe(710_000);
    expect(weeklySpend(1)[3]).toBe(0);
  });
});

describe('fleetAlerts', () => {
  const pending: DriverRequest = {
    id: 'R-1', orgId: 'o1', driverId: 'd1', vehicleId: 'f1', type: 'limit', amount: 200_000, reason: 'trip',
    note: { ar: '', en: '' }, status: 'pending', approved: 0, reply: { ar: '', en: '' }, createdAt: '', decidedAt: null,
  };

  it('lists every alert kind in the prototype order', () => {
    const alerts = fleetAlerts(
      {
        requests: [pending, { ...pending, id: 'R-2', status: 'approved' }],
        drivers: [drv({}), drv({ id: 'd2', active: false, name: { ar: 'عمر', en: 'Omar' } })],
        vehicles: [
          veh({ id: 'f1', active: false }),
          veh({ id: 'f2', driverId: null }),
          veh({ id: 'f3', tagBattery: 11 }),
        ],
        usage: { f2: 1_480_000, f3: 1_300_000 },
      },
      'en', T, N,
    );
    expect(alerts.map((a) => a.action.kind)).toEqual([
      'review-requests', 'reactivate-driver', 'reactivate-vehicle', 'raise-limit', 'raise-limit', 'order-tag',
    ]);
    expect(alerts[0].detail).toContain('200,000 SDG');
    expect(alerts[3]).toMatchObject({ color: 'rd', tag: 'Limit reached' });
    expect(alerts[3].detail).toContain('Unassigned');
    expect(alerts[4]).toMatchObject({ color: 'am', tag: 'Near limit' });
    expect(alerts[4].detail).toContain('200,000 SDG left');
  });

  it('suspended vehicles do not also raise limit alerts', () => {
    const alerts = fleetAlerts({ requests: [], drivers: [drv({})], vehicles: [veh({ active: false })], usage: { f1: 1_500_000 } }, 'en', T, N);
    expect(alerts).toHaveLength(1);
    expect(alerts[0].action.kind).toBe('reactivate-vehicle');
  });
});

describe('pumpState', () => {
  const tx = (pump: number, status: Transaction['status']) => ({ pump, status }) as Transaction;
  it('derives offline / busy / free', () => {
    expect(pumpState(false, 1, [])).toBe('off');
    expect(pumpState(true, 1, [tx(1, 'fueling')])).toBe('busy');
    expect(pumpState(true, 1, [tx(1, 'auth')])).toBe('busy');
    expect(pumpState(true, 1, [tx(1, 'done'), tx(2, 'auth')])).toBe('free');
  });
});
