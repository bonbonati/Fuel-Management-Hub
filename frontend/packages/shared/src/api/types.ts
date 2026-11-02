/**
 * API data types. These mirror the JSON returned by the Go services
 * (see backend/internal/<service>/model.go and service.go). Amounts are whole
 * Sudanese pounds (SDG); timestamps are ISO-8601 strings.
 */

export type Lang = 'ar' | 'en';

/** Bilingual text, Arabic first. */
export interface Text {
  ar: string;
  en: string;
}

/** Fuel grade keys: regular benzene, super benzene, diesel. */
export type Fuel = '91' | '95' | 'Diesel';
export const FUELS: Fuel[] = ['91', '95', 'Diesel'];

export type Role = 'driver' | 'fleet_manager' | 'attendant' | 'station_admin';

/** Sudanese number plate: digits plus the state code in Latin and Arabic. */
export interface Plate {
  digits: string;
  latin: string;
  arabic: string;
}

// ---------- auth ----------
export interface User {
  id: string;
  username: string;
  role: Role;
  orgId?: string;
  stationId?: string;
  actorId?: string;
  name: Text;
  active: boolean;
  demo: boolean;
}

export interface TokenResponse {
  accessToken: string;
  tokenType: 'Bearer';
  expiresAt: string;
  user: User;
}

// ---------- fleet ----------
export interface Org {
  id: string;
  name: Text;
  phone: string;
  city: Text;
}

export interface Tier {
  id: string;
  orgId: string;
  icon: string;
  color: string;
  name: Text;
  budget: number;
  dailyLitres: number;
  window: string;
}

export interface Driver {
  id: string;
  orgId: string;
  name: Text;
  phone: string;
  code: string;
  appActivated: boolean;
  active: boolean;
}

export interface Vehicle {
  id: string;
  orgId: string;
  plate: Plate;
  name: Text;
  driverId: string | null;
  fuel: Fuel;
  tierId: string | null;
  customLimit: number | null;
  extra: number;
  active: boolean;
  tagBattery: number;
  /** Custom limit or tier budget. */
  baseLimit: number;
  /** baseLimit + extra (computed by the fleet service). */
  limit: number;
}

export interface Rules {
  network: boolean;
  antiSiphon: boolean;
  geofence: boolean;
  wrongFuel: boolean;
  odometer: boolean;
  maxFill: boolean;
  maxFillAmount: number;
  hours: boolean;
  networkStationIds: string[];
}

export type RequestType = 'limit' | 'activate';
export type RequestStatus = 'pending' | 'approved' | 'rejected';
export type RequestReason = 'trip' | 'extra' | 'emergency' | 'review' | 'other';

export interface DriverRequest {
  id: string;
  orgId: string;
  driverId: string;
  vehicleId: string | null;
  type: RequestType;
  amount: number;
  reason: RequestReason;
  note: Text;
  status: RequestStatus;
  approved: number;
  reply: Text;
  createdAt: string;
  decidedAt: string | null;
}

export interface Block {
  kind: 'driver' | 'noveh' | 'veh';
  message: Text;
}

/** The driver's own context (GET /fleet/me). */
export interface DriverContext {
  orgId: string;
  orgName: Text;
  orgPhone: string;
  driver: Driver | null;
  vehicle: Vehicle | null;
  tier: Tier | null;
  rules: Rules;
  block: Block | null;
}

// ---------- stations ----------
export interface Station {
  id: string;
  name: Text;
  area: Text;
  distanceKm: number;
  wait: Text;
  phone: string;
  bankMask: string;
  pumpCount: number;
}

export interface Pump {
  stationId: string;
  number: number;
  grades: Fuel[];
  flowRate: number;
  online: boolean;
}

export interface Staff {
  id: string;
  stationId: string;
  name: Text;
  code: string;
  role: 'sup' | 'att';
  shift: 'A' | 'B' | 'C';
  device: string;
  active: boolean;
}

export interface StaffProfile {
  staff: Staff;
  station: Station;
}

export type Prices = Record<Fuel, number>;

// ---------- payments ----------
export interface Wallet {
  orgId: string;
  balance: number;
  month: string;
  /** vehicle id → SDG used this month */
  usage: Record<string, number>;
}

export interface Tax {
  enabled: boolean;
  rate: number;
}

export interface Settlement {
  stationId: string;
  date: string;
  amount: number;
  fee: number;
  status: 'scheduled' | 'paid';
}

export interface SettlementSummary {
  stationId: string;
  nextPayout: Settlement | null;
  feesThisMonth: number;
  items: Settlement[];
}

// ---------- fueling ----------
export type TxStatus = 'auth' | 'fueling' | 'done';
export type TxSource = 'app' | 'qr' | 'rfid';

export interface Transaction {
  id: string;
  orgId: string;
  vehicleId: string;
  driverId: string;
  plate: Plate;
  stationId: string;
  pump: number;
  fuel: Fuel;
  amount: number;
  pricePerLitre: number;
  litres: number;
  status: TxStatus;
  code?: string;
  source: TxSource;
  odometerKm: number | null;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
}

export interface AllowanceView {
  vehicleId: string;
  limit: number;
  used: number;
  remaining: number;
  available: number;
  reason: 'limit' | 'perfill';
}

export interface PassStatus {
  allowance: AllowanceView | null;
  odometer: { required: boolean; recorded: boolean; km: number | null };
  block: Block | null;
}

export interface PassToken {
  token: string;
  expiresAt: string;
  ttlSeconds: number;
}

export interface ScanResult {
  scanId: string;
  source: TxSource;
  token?: string;
  vehicle: { id: string; name: Text; plate: Plate; fuel: Fuel; active: boolean; limit: number };
  orgName: Text;
  driver: { id: string; name: Text; code: string; active: boolean } | null;
  available: number;
  reason: 'limit' | 'perfill';
  fuelLocked: boolean;
  active: boolean;
  block: Block | null;
}

export interface SecurityEvent {
  id: number;
  orgId: string;
  severity: 'critical' | 'warning';
  title: Text;
  detail: Text;
  ref: string;
  createdAt: string;
}
