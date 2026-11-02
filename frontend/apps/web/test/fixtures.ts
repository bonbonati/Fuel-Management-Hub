/**
 * Test fixtures and a fetch-level fake of the API gateway. Tests run the real
 * app (providers, data hooks, API client) against these routes, so they cover
 * the same code paths as production without a backend.
 */
import { vi } from 'vitest';
import type { Driver, DriverRequest, Pump, Rules, Staff, Station, Tier, Transaction, User, Vehicle } from '@petro/shared';

const plate = (digits: string, latin: string, arabic: string) => ({ digits, latin, arabic });

export const users: Record<string, User> = {
  fm: { id: 'u1', username: 'fleet.manager', role: 'fleet_manager', orgId: 'o1', actorId: 'm1', name: { ar: 'مدير', en: 'Fleet manager' }, active: true, demo: true },
  sa: { id: 'u2', username: 'ATT-01', role: 'station_admin', stationId: 's1', actorId: 'a1', name: { ar: 'مجدي', en: 'Magdi' }, active: true, demo: true },
  drv: { id: 'u3', username: 'D-3071', role: 'driver', orgId: 'o1', actorId: 'd3', name: { ar: 'عبدالرحمن', en: 'Abdurahman' }, active: true, demo: true },
};

export const tiers: Tier[] = [
  { id: 't1', orgId: 'o1', icon: 'truck', color: 'or', name: { ar: 'شاحنات النقل', en: 'Freight trucks' }, budget: 1500000, dailyLitres: 200, window: '24/7' },
  { id: 't2', orgId: 'o1', icon: 'bus', color: 'bl', name: { ar: 'حافلات التوصيل', en: 'Delivery vans' }, budget: 1200000, dailyLitres: 80, window: '06:00–21:00' },
];

export const drivers: Driver[] = [
  { id: 'd1', orgId: 'o1', name: { ar: 'فيصل عثمان', en: 'Faisal Osman' }, phone: '+249 91 234 2114', code: 'D-3012', appActivated: true, active: true },
  { id: 'd3', orgId: 'o1', name: { ar: 'عبدالرحمن حسن', en: 'Abdurahman Hassan' }, phone: '+249 90 552 4471', code: 'D-3071', appActivated: true, active: true },
];

export const vehicles: Vehicle[] = [
  { id: 'f1', orgId: 'o1', plate: plate('4410', 'KRT', 'خ ر ط'), name: { ar: 'إيسوزو دي ماكس', en: 'Isuzu D-Max' }, driverId: 'd1', fuel: 'Diesel', tierId: 't1', customLimit: null, extra: 0, active: true, tagBattery: 92, baseLimit: 1500000, limit: 1500000 },
  { id: 'f3', orgId: 'o1', plate: plate('7781', 'KRT', 'خ ر ط'), name: { ar: 'هيونداي H1', en: 'Hyundai H1' }, driverId: 'd3', fuel: '95', tierId: 't2', customLimit: null, extra: 150000, active: true, tagBattery: 11, baseLimit: 1200000, limit: 1350000 },
];

export const rules: Rules = { network: true, antiSiphon: true, geofence: true, wrongFuel: true, odometer: true, maxFill: true, maxFillAmount: 400000, hours: true, networkStationIds: ['s1', 's2'] };

export const requests: DriverRequest[] = [
  { id: 'R-1043', orgId: 'o1', driverId: 'd1', vehicleId: 'f1', type: 'limit', amount: 200000, reason: 'trip', note: { ar: 'رحلة', en: 'Trip to Wad Madani' }, status: 'pending', approved: 0, reply: { ar: '', en: '' }, createdAt: new Date().toISOString(), decidedAt: null },
];

export const stations: Station[] = [
  { id: 's1', name: { ar: 'محطة النيل الأزرق', en: 'Blue Nile Station' }, area: { ar: 'شارع أفريقيا', en: 'Africa Rd' }, distanceKm: 1.2, wait: { ar: '', en: 'No queue' }, phone: '+249 18 377 0001', bankMask: '0917', pumpCount: 2 },
];

export const pumps: Pump[] = [
  { stationId: 's1', number: 1, grades: ['91', '95'], flowRate: 40, online: true },
  { stationId: 's1', number: 7, grades: ['Diesel'], flowRate: 120, online: false },
];

export const staff: Staff[] = [
  { id: 'a1', stationId: 's1', name: { ar: 'مجدي الشيخ', en: 'Magdi Alsheikh' }, code: 'ATT-01', role: 'sup', shift: 'A', device: 'iPhone 13', active: true },
];

export const tx: Transaction[] = [
  { id: 'T-90412', orgId: 'o1', vehicleId: 'f3', driverId: 'd3', plate: plate('7781', 'KRT', 'خ ر ط'), stationId: 's1', pump: 3, fuel: '95', amount: 180000, pricePerLitre: 3600, litres: 50, status: 'done', source: 'qr', odometerKm: null, createdAt: new Date().toISOString(), startedAt: null, completedAt: null },
  { id: 'T-90413', orgId: 'o1', vehicleId: 'f1', driverId: 'd1', plate: plate('4410', 'KRT', 'خ ر ط'), stationId: 's1', pump: 1, fuel: 'Diesel', amount: 100000, pricePerLitre: 3250, litres: 30.8, status: 'auth', code: '4821', source: 'app', odometerKm: 142880, createdAt: new Date().toISOString(), startedAt: null, completedAt: null },
];

type Handler = (body: unknown, url: URL) => unknown;

/**
 * Installs a fake gateway. Routes are "METHOD /path" (query ignored). Returns
 * the vi.fn so tests can assert on calls. Unknown routes answer 404.
 */
export function fakeGateway(overrides: Record<string, Handler> = {}) {
  const routes: Record<string, Handler> = {
    'GET /api/v1/auth/demo-accounts': () => [users.fm, users.sa, users.drv],
    'POST /api/v1/auth/demo-login': (b) => {
      const u = Object.values(users).find((x) => x.username === (b as { username: string }).username)!;
      return { accessToken: 'tok-' + u.username, tokenType: 'Bearer', expiresAt: '2999-01-01T00:00:00Z', user: u };
    },
    'GET /api/v1/fleet/org': () => ({ id: 'o1', name: { ar: 'شركة النماء', en: 'Al Namaa Logistics Co.' }, phone: '+249 91 230 4471', city: { ar: 'الخرطوم', en: 'Khartoum' } }),
    'GET /api/v1/fleet/tiers': () => tiers,
    'GET /api/v1/fleet/drivers': () => drivers,
    'GET /api/v1/fleet/vehicles': () => vehicles,
    'GET /api/v1/fleet/rules': () => rules,
    'GET /api/v1/fleet/requests': () => requests,
    'GET /api/v1/stations': () => stations,
    'GET /api/v1/stations/fuel-prices': () => ({ '91': 3420, '95': 3600, Diesel: 3250 }),
    'GET /api/v1/stations/staff/me': () => ({ staff: staff[0], station: stations[0] }),
    'GET /api/v1/stations/s1/pumps': () => pumps,
    'GET /api/v1/stations/s1/staff': () => staff,
    'GET /api/v1/payments/wallet': () => ({ orgId: 'o1', balance: 18450000, month: '2026-11', usage: { f1: 1236000, f3: 768000 } }),
    'GET /api/v1/payments/tax': () => ({ enabled: true, rate: 17 }),
    'GET /api/v1/payments/settlements': () => ({ stationId: 's1', nextPayout: { stationId: 's1', date: '2026-11-03', amount: 27630000, fee: 119500, status: 'scheduled' }, feesThisMonth: 468600, items: [] }),
    'GET /api/v1/fueling/transactions': () => tx,
    'GET /api/v1/fueling/security-events': () => [],
    ...overrides,
  };
  const fn = vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(input, 'http://localhost');
    const key = `${init?.method ?? 'GET'} ${url.pathname}`;
    const h = routes[key];
    if (!h) return new Response(JSON.stringify({ code: 'not_found', message: { ar: 'غير موجود', en: 'Not found' } }), { status: 404 });
    const body = init?.body ? JSON.parse(init.body as string) : undefined;
    try {
      return new Response(JSON.stringify(h(body, url) ?? {}), { status: 200 });
    } catch (e) {
      const p = e as { status: number; code: string; message: { ar: string; en: string } };
      return new Response(JSON.stringify(p), { status: p.status ?? 500 });
    }
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

/** Signs a persona in by storing a session, as a returning user would have. */
export function signIn(u: User) {
  localStorage.setItem('petro-web-session', JSON.stringify({ token: 'tok', user: u, expiresAt: '2999-01-01T00:00:00Z' }));
  localStorage.setItem('petro-lang', 'en');
}

/** Calls made to a route, with parsed bodies. */
export function callsTo(fetch: ReturnType<typeof fakeGateway>, method: string, path: string) {
  return fetch.mock.calls
    .filter(([u, i]) => new URL(u as string, 'http://localhost').pathname === path && ((i as RequestInit | undefined)?.method ?? 'GET') === method)
    .map(([, i]) => ((i as RequestInit).body ? JSON.parse((i as RequestInit).body as string) : undefined));
}
