/**
 * Web admin root: sign-in gate, top bar and the console shell with its two
 * portals — fleet (fleet managers) and station (station supervisors). The
 * portal is decided by the signed-in role; in demo mode the portal switch
 * signs in as the other persona.
 */
import { useQuery } from '@tanstack/react-query';
import {
  api, Icon, Login, ROLE_LABEL, useAlerts, useFleetData, useI18n, useOrg, useSession, useStaff, useStationData, type Role,
} from '@petro/shared';
import type { ReactNode } from 'react';
import { FleetAlerts, FleetDrivers, FleetOverview, FleetRequests, FleetRules, FleetTiers, FleetTx, FleetVehicles } from './fleet';
import { driverSpec, EditModal, ModalProvider, pumpSpec, staffSpec, tierSpec, useSpecCtx, vehicleSpec } from './modals';
import { Settings, StOverview, StPumps, StSettle, StStaff, StTx } from './station';
import { useHashTab } from './ui';

const WEB_ROLES: Role[] = ['fleet_manager', 'station_admin'];

export function App() {
  const { user } = useSession();
  const { T } = useI18n();
  return (
    <>
      <Topbar />
      {!user ? (
        <Login roles={WEB_ROLES} title={T('لوحة تحكم بترو', 'Petro web admin')}
          subtitle={T('لمديري الأساطيل ومشرفي المحطات.', 'For fleet managers and station supervisors.')} />
      ) : user.role === 'fleet_manager' ? (
        <FleetConsole />
      ) : user.role === 'station_admin' ? (
        <StationConsole />
      ) : (
        <WrongApp role={user.role} />
      )}
    </>
  );
}

function Topbar() {
  const { T, N, lang, toggleLang } = useI18n();
  const { user, logout } = useSession();
  return (
    <header className="topbar">
      <span className="brand">
        <span className="mark"><Icon name="pump" size={18} /></span>
        {T('بترو', 'Petro')}
        <span className="muted" style={{ fontSize: 13, fontWeight: 500, fontFamily: 'var(--body)' }}>PetroFleet · {T('السودان', 'Sudan')}</span>
      </span>
      {user && <span className="sync"><span className="dot pulse" />{T('متزامن مع تطبيقات الجوال', 'Synced with mobile apps')}</span>}
      <div className="tb-acts">
        <button className="lang" onClick={toggleLang} lang={lang === 'ar' ? 'en' : 'ar'}>{lang === 'ar' ? 'English' : 'العربية'}</button>
        {user && (
          <>
            <span className="whoami">{N(user.name)}</span>
            <button className="lang" onClick={logout} aria-label={T('تسجيل الخروج', 'Sign out')}><Icon name="logout" size={15} /></button>
          </>
        )}
      </div>
    </header>
  );
}

function WrongApp({ role }: { role: Role }) {
  const { T } = useI18n();
  const { logout } = useSession();
  return (
    <div className="login">
      <div className="card">
        <h1>{T(...ROLE_LABEL[role])}</h1>
        <p className="sub">{T('هذا الحساب مخصص لتطبيق الجوال، وهو قيد التطوير.', 'This account is for the mobile app, which is under development.')}</p>
        <button className="btn-s" onClick={logout}>{T('تسجيل الخروج', 'Sign out')}</button>
      </div>
    </div>
  );
}

/** Portal switch: in demo mode, signs in as the other portal's persona. */
function PortalSwitch({ current }: { current: 'fleet' | 'station' }) {
  const { T } = useI18n();
  const { demoLogin } = useSession();
  const demo = useQuery({ queryKey: ['demo-accounts'], queryFn: api.auth.demoAccounts, refetchInterval: false });
  const target = (role: Role) => demo.data?.find((u) => u.role === role);
  const fm = target('fleet_manager'), sa = target('station_admin');
  if (!fm || !sa) return null;
  return (
    <div className="portal" role="group" aria-label={T('البوابة', 'Portal')}>
      <button aria-pressed={current === 'fleet'} style={current === 'fleet' ? { background: 'var(--bl)' } : undefined}
        onClick={() => current !== 'fleet' && void demoLogin(fm.username)}>{T('إدارة الأسطول', 'Fleet admin')}</button>
      <button aria-pressed={current === 'station'} style={current === 'station' ? { background: 'var(--or)' } : undefined}
        onClick={() => current !== 'station' && void demoLogin(sa.username)}>{T('إدارة المحطة', 'Station admin')}</button>
    </div>
  );
}

interface NavItem<K extends string> { k: K; l: string; i: string; cnt?: number }

function Shell<K extends string>({ portal, org, sub, nav, tab, go, children }: {
  portal: 'fleet' | 'station'; org: ReactNode; sub: ReactNode; nav: NavItem<K>[]; tab: K; go: (k: K) => void; children: ReactNode;
}) {
  const { T } = useI18n();
  return (
    <div className="admin" data-r={portal}>
      <aside className="sidebar">
        <div className="org">
          <span className="ava sm">{portal === 'fleet' ? T('ن', 'AN') : T('ن ز', 'BN')}</span>
          <div style={{ minWidth: 0 }}><b>{org}</b><span>{sub}</span></div>
        </div>
        <PortalSwitch current={portal} />
        <nav className="menu">
          {nav.map((n) => (
            <button key={n.k} aria-current={n.k === tab} onClick={() => go(n.k)}>
              <Icon name={n.i} />{n.l}{n.cnt ? <span className="cnt">{n.cnt}</span> : null}
            </button>
          ))}
        </nav>
        <div className="side-foot">{T('تنعكس كل التغييرات فوراً على تطبيقات السائقين والعاملين.', "All changes apply instantly to the drivers' and attendants' apps.")}</div>
      </aside>
      <section className="main">{children}</section>
    </div>
  );
}

const Loading = () => <div className="loading"><Icon name="refresh" size={22} /></div>;

const FLEET_TABS = ['overview', 'requests', 'vehicles', 'drivers', 'tiers', 'rules', 'tx', 'alerts', 'settings'] as const;
type FleetTab = (typeof FLEET_TABS)[number];

function FleetConsole() {
  const { T, N } = useI18n();
  const d = useFleetData();
  const ctx = useSpecCtx();
  const [tab, go] = useHashTab<FleetTab>(FLEET_TABS, 'overview');
  const alerts = useAlerts(d);
  const pending = d.requests.filter((r) => r.status === 'pending').length;
  const nav: NavItem<FleetTab>[] = [
    { k: 'overview', l: T('نظرة عامة', 'Overview'), i: 'dash' },
    { k: 'requests', l: T('طلبات السائقين', 'Driver requests'), i: 'inbox', cnt: pending },
    { k: 'vehicles', l: T('المركبات', 'Vehicles'), i: 'truck' },
    { k: 'drivers', l: T('السائقون', 'Drivers'), i: 'users' },
    { k: 'tiers', l: T('الفئات والميزانيات', 'Tiers & budgets'), i: 'layers' },
    { k: 'rules', l: T('قواعد الوقود', 'Fuel rules'), i: 'shield' },
    { k: 'tx', l: T('العمليات', 'Transactions'), i: 'receipt' },
    { k: 'alerts', l: T('التنبيهات', 'Alerts'), i: 'bell', cnt: alerts.length },
    { k: 'settings', l: T('الإعدادات والضريبة', 'Settings & VAT'), i: 'cog' },
  ];
  const goAny = (t: string) => go(t as FleetTab);
  const orgName = N(useOrg().data?.name);
  return (
    <ModalProvider render={(m, close) => {
      const spec = m.type === 'vehicle' ? vehicleSpec(m.id, d, ctx) : m.type === 'driver' ? driverSpec(m.id, d, ctx) : m.type === 'tier' ? tierSpec(m.id, d, ctx) : null;
      return spec && <EditModal spec={spec} onClose={close} />;
    }}>
      <Shell portal="fleet" org={orgName} sub={T(`حساب أسطول · ${d.vehicles.length} مركبات`, `Fleet account · ${d.vehicles.length} vehicles`)} nav={nav} tab={tab} go={go}>
        {d.loading ? <Loading /> : {
          overview: <FleetOverview d={d} go={goAny} />,
          requests: <FleetRequests d={d} />,
          vehicles: <FleetVehicles d={d} />,
          drivers: <FleetDrivers d={d} />,
          tiers: <FleetTiers d={d} />,
          rules: <FleetRules d={d} />,
          tx: <FleetTx d={d} go={goAny} />,
          alerts: <FleetAlerts d={d} go={goAny} />,
          settings: <Settings portal="fleet" />,
        }[tab]}
      </Shell>
    </ModalProvider>
  );
}

const STATION_TABS = ['overview', 'pumps', 'tx', 'staff', 'settle', 'settings'] as const;
type StationTab = (typeof STATION_TABS)[number];

function StationConsole() {
  const { T, N } = useI18n();
  const s = useStationData();
  const ctx = useSpecCtx();
  const staff = useStaff(s.stationId).data ?? [];
  const [tab, go] = useHashTab<StationTab>(STATION_TABS, 'overview');
  const pending = s.tx.filter((t) => t.status !== 'done').length;
  const nav: NavItem<StationTab>[] = [
    { k: 'overview', l: T('نظرة عامة', 'Overview'), i: 'dash', cnt: pending },
    { k: 'pumps', l: T('المضخات', 'Pumps'), i: 'pump' },
    { k: 'tx', l: T('العمليات', 'Transactions'), i: 'receipt' },
    { k: 'staff', l: T('الموظفون والورديات', 'Staff & shifts'), i: 'users' },
    { k: 'settle', l: T('التسويات', 'Settlements'), i: 'bank' },
    { k: 'settings', l: T('الإعدادات والضريبة', 'Settings & VAT'), i: 'cog' },
  ];
  return (
    <ModalProvider render={(m, close) => {
      const pump = m.type === 'pump' ? s.pumps.find((p) => String(p.number) === m.id) : undefined;
      const spec = m.type === 'staff' ? staffSpec(m.id, s, ctx, staff) : pump ? pumpSpec(pump, ctx) : null;
      return spec && <EditModal spec={spec} onClose={close} />;
    }}>
      <Shell portal="station" org={N(s.station?.name)} sub={T(`محطة شريكة · ${s.pumps.length} مضخات`, `Partner station · ${s.pumps.length} pumps`)} nav={nav} tab={tab} go={go}>
        {s.loading ? <Loading /> : {
          overview: <StOverview s={s} />,
          pumps: <StPumps s={s} />,
          tx: <StTx s={s} go={(t) => go(t as StationTab)} />,
          staff: <StStaff s={s} />,
          settle: <StSettle s={s} />,
          settings: <Settings portal="station" s={s} />,
        }[tab]}
      </Shell>
    </ModalProvider>
  );
}
