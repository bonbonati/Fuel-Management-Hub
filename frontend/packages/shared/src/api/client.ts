/**
 * Typed HTTP client for the PetroFleet API gateway.
 *
 * Every service is reached through the same gateway under /api/v1/<service>.
 * Errors come back as RFC 9457 problem documents with a stable `code` and a
 * bilingual `message`; they are surfaced as ApiError so screens can show the
 * server's own wording in the user's language.
 */
import type {
  Driver, DriverContext, DriverRequest, Fuel, Org, PassStatus, PassToken, Prices, Pump,
  RequestReason, RequestType, Rules, ScanResult, SecurityEvent, SettlementSummary, Staff, StaffProfile,
  Station, Tax, Text, Tier, TokenResponse, Transaction, User, Vehicle, Wallet, Plate,
} from './types';

/** An error response from the API (or a network failure, status 0). */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly text: Text;

  constructor(status: number, code: string, text: Text) {
    super(text.en || code);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.text = text;
  }
}

const NETWORK_ERROR: Text = {
  ar: 'تعذّر الاتصال بالخادم. تحقق من الاتصال وحاول مرة أخرى.',
  en: 'Could not reach the server. Check your connection and try again.',
};

export interface ApiConfig {
  /** Gateway origin, '' for same-origin (web) or e.g. https://api.example.com (native app). */
  baseUrl: string;
  /** Returns the current access token, if signed in. */
  getToken: () => string | null;
  /** Called on 401 so the app can drop the session. */
  onUnauthorized?: () => void;
}

let config: ApiConfig = { baseUrl: '', getToken: () => null };

/** Configures the shared client (call once at app start-up). */
export function configureApi(c: ApiConfig): void {
  config = c;
}

/** Performs a JSON request against the gateway. */
export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const token = config.getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  let res: Response;
  try {
    res = await fetch(config.baseUrl + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  } catch {
    throw new ApiError(0, 'network', NETWORK_ERROR);
  }
  const raw = await res.text();
  const data = raw ? safeJson(raw) : undefined;
  if (!res.ok) {
    if (res.status === 401 && token) config.onUnauthorized?.();
    const p = (data ?? {}) as { code?: string; message?: Text };
    throw new ApiError(res.status, p.code ?? 'http_' + res.status, p.message ?? NETWORK_ERROR);
  }
  return data as T;
}

function safeJson(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}

const get = <T>(p: string) => request<T>('GET', p);
const post = <T>(p: string, b?: unknown) => request<T>('POST', p, b ?? {});
const put = <T>(p: string, b: unknown) => request<T>('PUT', p, b);
const patch = <T>(p: string, b: unknown) => request<T>('PATCH', p, b);
const enc = encodeURIComponent;

// ---------- inputs ----------
export interface TierInput { name: Text; budget: number; dailyLitres: number; window: string; vehicleIds: string[] }
export interface DriverInput { name: Text; code: string; phone: string; appActivated: boolean; active: boolean; vehicleId: string | null }
export interface VehicleInput {
  plate: Plate; name: Text; fuel: Fuel; tierId: string | null; customLimit: number | null;
  driverId: string | null; tagBattery: number; active: boolean;
}
export interface StaffInput { name: Text; code: string; role: 'sup' | 'att'; shift: 'A' | 'B' | 'C'; device: string; active: boolean }
export interface PumpInput { grades: Fuel[]; flowRate: number; online: boolean }

/** Every API operation, grouped by service. */
export const api = {
  auth: {
    login: (username: string, password: string) => post<TokenResponse>('/api/v1/auth/login', { username, password }),
    demoLogin: (username: string) => post<TokenResponse>('/api/v1/auth/demo-login', { username }),
    demoAccounts: () => get<User[]>('/api/v1/auth/demo-accounts'),
    me: () => get<User>('/api/v1/auth/me'),
  },
  fleet: {
    org: () => get<Org>('/api/v1/fleet/org'),
    updateOrg: (p: { name?: Text; phone?: string }) => patch<Org>('/api/v1/fleet/org', p),
    tiers: () => get<Tier[]>('/api/v1/fleet/tiers'),
    saveTier: (id: string | null, t: TierInput) =>
      id ? put<Tier>(`/api/v1/fleet/tiers/${enc(id)}`, t) : post<Tier>('/api/v1/fleet/tiers', t),
    drivers: () => get<Driver[]>('/api/v1/fleet/drivers'),
    saveDriver: (id: string | null, d: DriverInput) =>
      id ? put<Driver>(`/api/v1/fleet/drivers/${enc(id)}`, d) : post<Driver>('/api/v1/fleet/drivers', d),
    setDriverActive: (id: string, active: boolean) => patch<Driver>(`/api/v1/fleet/drivers/${enc(id)}/status`, { active }),
    vehicles: () => get<Vehicle[]>('/api/v1/fleet/vehicles'),
    saveVehicle: (id: string | null, v: VehicleInput) =>
      id ? put<Vehicle>(`/api/v1/fleet/vehicles/${enc(id)}`, v) : post<Vehicle>('/api/v1/fleet/vehicles', v),
    setVehicleActive: (id: string, active: boolean) => patch<Vehicle>(`/api/v1/fleet/vehicles/${enc(id)}/status`, { active }),
    setVehicleTier: (id: string, tierId: string | null) => patch<Vehicle>(`/api/v1/fleet/vehicles/${enc(id)}/tier`, { tierId }),
    increaseLimit: (id: string, amount: number) => post<Vehicle>(`/api/v1/fleet/vehicles/${enc(id)}/limit-increases`, { amount }),
    rules: () => get<Rules>('/api/v1/fleet/rules'),
    patchRules: (p: Partial<Rules>) => patch<Rules>('/api/v1/fleet/rules', p),
    requests: (status?: string) => get<DriverRequest[]>('/api/v1/fleet/requests' + (status ? `?status=${enc(status)}` : '')),
    createRequest: (r: { type: RequestType; amount: number; reason: RequestReason; note: string }) =>
      post<DriverRequest>('/api/v1/fleet/requests', r),
    decide: (id: string, d: { approve: boolean; amount?: number; note?: string }) =>
      post<DriverRequest>(`/api/v1/fleet/requests/${enc(id)}/decision`, d),
    me: () => get<DriverContext>('/api/v1/fleet/me'),
  },
  stations: {
    list: () => get<Station[]>('/api/v1/stations'),
    get: (id: string) => get<Station>(`/api/v1/stations/${enc(id)}`),
    update: (id: string, p: { name?: Partial<Text>; area?: Partial<Text>; phone?: string }) =>
      patch<Station>(`/api/v1/stations/${enc(id)}`, p),
    prices: () => get<Prices>('/api/v1/stations/fuel-prices'),
    pumps: (id: string) => get<Pump[]>(`/api/v1/stations/${enc(id)}/pumps`),
    updatePump: (id: string, n: number, p: PumpInput) => put<Pump>(`/api/v1/stations/${enc(id)}/pumps/${n}`, p),
    setPumpOnline: (id: string, n: number, online: boolean) => patch<Pump>(`/api/v1/stations/${enc(id)}/pumps/${n}/status`, { online }),
    staff: (id: string) => get<Staff[]>(`/api/v1/stations/${enc(id)}/staff`),
    saveStaff: (id: string, sid: string | null, s: StaffInput) =>
      sid ? put<Staff>(`/api/v1/stations/${enc(id)}/staff/${enc(sid)}`, s) : post<Staff>(`/api/v1/stations/${enc(id)}/staff`, s),
    setStaffActive: (id: string, sid: string, active: boolean) =>
      patch<Staff>(`/api/v1/stations/${enc(id)}/staff/${enc(sid)}/status`, { active }),
    me: () => get<StaffProfile>('/api/v1/stations/staff/me'),
  },
  payments: {
    wallet: () => get<Wallet>('/api/v1/payments/wallet'),
    topUp: (amount: number) => post<Wallet>('/api/v1/payments/wallet/topups', { amount }),
    tax: () => get<Tax>('/api/v1/payments/tax'),
    setTax: (t: Tax) => put<Tax>('/api/v1/payments/tax', t),
    settlements: () => get<SettlementSummary>('/api/v1/payments/settlements'),
  },
  fueling: {
    pass: () => get<PassStatus>('/api/v1/fueling/me/pass'),
    passToken: () => post<PassToken>('/api/v1/fueling/me/pass-token'),
    odometer: (km: string) => post<PassStatus['odometer']>('/api/v1/fueling/me/odometer', { km }),
    authorize: (a: { stationId: string; pump: number; fuel: Fuel; amount: number }) =>
      post<Transaction>('/api/v1/fueling/authorizations', a),
    scan: (s: { method: 'qr'; token: string } | { method: 'rfid'; vehicleId: string }) => post<ScanResult>('/api/v1/fueling/scans', s),
    dispense: (d: { scanId: string; pump: number; amount?: number; litres?: number }) => post<Transaction>('/api/v1/fueling/dispenses', d),
    start: (id: string) => post<Transaction>(`/api/v1/fueling/transactions/${enc(id)}/start`),
    startByCode: (code: string) => post<Transaction>('/api/v1/fueling/transactions/start-by-code', { code }),
    finish: (id: string) => post<Transaction>(`/api/v1/fueling/transactions/${enc(id)}/finish`),
    transactions: () => get<Transaction[]>('/api/v1/fueling/transactions'),
    securityEvents: () => get<SecurityEvent[]>('/api/v1/fueling/security-events'),
  },
};

