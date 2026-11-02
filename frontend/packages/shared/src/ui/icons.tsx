/**
 * Icon set: the prototype's short icon keys mapped to Lucide components.
 * Only the icons listed here are bundled (explicit imports keep tree-shaking).
 */
import type { LucideIcon } from 'lucide-react';
import {
  ArrowLeftRight, BatteryLow, Bell, Building2, Bus, Camera, CarFront, ChartColumn, Check, ChevronLeft, CircleAlert,
  CircleCheck, CircleUserRound, CircleX, ClipboardPaste, Clock, Copy, Download, Droplet, Fingerprint, Fuel, Gauge,
  HandCoins, History, Hourglass, Inbox, Info, KeyRound, Landmark, Languages, Layers, LayoutDashboard, ListChecks, Lock,
  LockOpen, LogOut, MapPin, Navigation, Network, Nfc, Pencil, Percent, Phone, Plus, Power, QrCode, ReceiptText,
  RefreshCw, ScanLine, Search, Send, Settings, ShieldAlert, ShieldCheck, Smartphone, Timer, TrendingUp, Truck,
  UserCheck, Users, Wallet, X,
} from 'lucide-react';

export const ICONS: Record<string, LucideIcon> = {
  wallet: Wallet, clock: Clock, history: History, truck: Truck, car: CarFront, bus: Bus, user: CircleUserRound,
  users: Users, pump: Fuel, dash: LayoutDashboard, bell: Bell, list: ListChecks, receipt: ReceiptText, check: Check,
  checkc: CircleCheck, pin: MapPin, key: KeyRound, shield: ShieldCheck, bank: Landmark, plus: Plus, edit: Pencil,
  download: Download, qr: QrCode, rfid: Nfc, camera: Camera, route: Navigation, swap: ArrowLeftRight, gauge: Gauge,
  battery: BatteryLow, lock: Lock, finger: Fingerprint, power: Power, nozzle: Droplet, layers: Layers, refresh: RefreshCw,
  grid: Network, phone: Phone, cog: Settings, percent: Percent, chart: ChartColumn, x: X, inbox: Inbox, send: Send,
  xc: CircleX, up: TrendingUp, hour: Hourglass, unlock: LockOpen, info: Info, coins: HandCoins, alert: CircleAlert,
  building: Building2, lang: Languages, timer: Timer, search: Search, scan: ScanLine, siren: ShieldAlert,
  ucheck: UserCheck, chev: ChevronLeft, logout: LogOut, mobile: Smartphone, copy: Copy, paste: ClipboardPaste,
};

/** Renders an icon by key (unknown keys fall back to "info"). */
export function Icon({ name, size }: { name: string; size?: number }) {
  const C = ICONS[name] ?? Info;
  return <C className="i" aria-hidden="true" strokeWidth={1.8} style={size ? { width: size, height: size } : undefined} />;
}
