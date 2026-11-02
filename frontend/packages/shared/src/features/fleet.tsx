/**
 * Fleet-manager building blocks used by both the web console and the mobile
 * fleet app (prototype: reqCardFleet, reqFilterBar, tierCard, rulesBlock,
 * secLogCard, alertCard, and the fleet data they read).
 */
import { useState } from 'react';
import { api } from '../api/client';
import type { DriverRequest, Rules, SecurityEvent, Tier } from '../api/types';
import { fleetAlerts, quotaCls, REASON, REQTYPE, tierStats, usedOf, type Alert } from '../domain';
import { agoLabel, curTxt, initials, pair, plateTxt, sar, short, whenLabel } from '../format';
import { useI18n } from '../i18n';
import {
  useAction, useDrivers, usePrices, useRequests, useRules, useSecurityEvents, useStations, useTax, useTiers,
  useTransactions, useVehicles, useWallet,
} from '../queries';
import { useToast } from '../toast';
import { PlateTag, Quota, ReqPill, Toggle } from '../ui/atoms';
import { Icon } from '../ui/icons';
import { DEFAULT_PRICES } from '../domain';

/** Everything the fleet screens read, with safe defaults while loading. */
export function useFleetData() {
  const vehicles = useVehicles(), drivers = useDrivers(), tiers = useTiers(), wallet = useWallet(), requests = useRequests();
  const rules = useRules(), tax = useTax(), prices = usePrices(), stations = useStations(), tx = useTransactions(), security = useSecurityEvents();
  return {
    vehicles: vehicles.data ?? [],
    drivers: drivers.data ?? [],
    tiers: tiers.data ?? [],
    wallet: wallet.data,
    usage: wallet.data?.usage,
    requests: requests.data ?? [],
    rules: rules.data,
    tax: tax.data,
    prices: prices.data ?? DEFAULT_PRICES,
    stations: stations.data ?? [],
    tx: tx.data ?? [],
    security: security.data ?? [],
    loading: !vehicles.data || !drivers.data || !tiers.data || !wallet.data || !requests.data || !rules.data,
  };
}
export type FleetData = ReturnType<typeof useFleetData>;

/** Name of a vehicle's driver, or "Unassigned". */
export function useDriverName(data: FleetData) {
  const { T, N } = useI18n();
  return (driverId: string | null | undefined) => {
    const d = driverId ? data.drivers.find((x) => x.id === driverId) : undefined;
    return d ? N(d.name) : T('غير مخصص', 'Unassigned');
  };
}

export type ReqFilter = 'pending' | 'approved' | 'rejected' | 'all';

export function ReqFilterBar({ value, onChange, requests }: { value: ReqFilter; onChange: (f: ReqFilter) => void; requests: DriverRequest[] }) {
  const { T } = useI18n();
  const count = (k: ReqFilter) => requests.filter((r) => k === 'all' || r.status === k).length;
  const opts: [ReqFilter, string][] = [
    ['pending', T('قيد المراجعة', 'Under review')], ['approved', T('معتمدة', 'Approved')],
    ['rejected', T('مرفوضة', 'Declined')], ['all', T('الكل', 'All')],
  ];
  return (
    <div className="chips" style={{ marginBottom: 12 }}>
      {opts.map(([k, l]) => (
        <button key={k} className="chipsel" aria-pressed={value === k} onClick={() => onChange(k)}
          style={value === k ? { background: 'var(--bl)', borderColor: 'var(--bl)', color: '#fff' } : undefined}>
          {l} <span className="num">{count(k)}</span>
        </button>
      ))}
    </div>
  );
}

/** A driver request with approve/decline controls while pending. */
export function ReqCardFleet({ r, data }: { r: DriverRequest; data: FleetData }) {
  const { T, N, lang } = useI18n();
  const run = useAction();
  const toast = useToast();
  const [amount, setAmount] = useState(String(r.amount));
  const [note, setNote] = useState('');
  const d = data.drivers.find((x) => x.id === r.driverId);
  const v = r.vehicleId ? data.vehicles.find((x) => x.id === r.vehicleId) : undefined;
  const t = v?.tierId ? data.tiers.find((x) => x.id === v.tierId) : undefined;
  const pend = r.status === 'pending';
  const color = r.type === 'limit' ? 'bl' : 'vi';

  const decide = (approve: boolean) => {
    let amt: number | undefined;
    if (approve && r.type === 'limit') {
      amt = Number(amount);
      if (!(amt >= 10000)) {
        toast(T('أدخل مبلغاً معتمداً لا يقل عن 10,000 ج.س', 'Enter an approved amount of at least SDG 10,000'));
        return;
      }
    }
    const name = d ? N(d.name) : '';
    void run(() => api.fleet.decide(r.id, { approve, amount: amt, note: note.trim() }), (res) =>
      !approve
        ? T('تم رفض الطلب وإشعار السائق', 'Request declined and driver notified')
        : r.type === 'limit'
          ? T(`تم اعتماد ${curTxt(res.approved, lang)} لـ ${name}`, `${curTxt(res.approved, lang)} approved for ${name}`)
          : T(`تمت إعادة تفعيل ${name}`, `${name} reactivated`));
  };

  return (
    <div className="card req">
      <div className="hd">
        <div style={{ display: 'flex', gap: 10, minWidth: 0 }}>
          <span className="ava sm" style={{ background: `var(--${color})` }}>{d ? initials(N(d.name), lang) : ''}</span>
          <div style={{ minWidth: 0 }}>
            <b className="w" style={{ display: 'block' }}>{d ? N(d.name) : ''}</b>
            <span className="muted num" style={{ fontSize: 12 }}>{r.id} · {whenLabel(r.createdAt, lang)}</span>
          </div>
        </div>
        <ReqPill r={r} />
      </div>
      <div className="meta">
        <span className={`pill ${color}`}><Icon name={r.type === 'limit' ? 'up' : 'unlock'} size={13} />{N(REQTYPE[r.type])}</span>
        {r.type === 'limit' && <span className="pill"><span className="num">{sar(r.amount)}</span> {T('ج.س', 'SDG')}</span>}
        <span className="pill">{N(REASON[r.reason])}</span>
      </div>
      {v && (
        <div className="deep" style={{ fontSize: 12, display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <span><PlateTag p={v.plate} /> <span className="muted">{N(v.name)}{t ? ' · ' + N(t.name) : ''}</span></span>
          <span className="num">{pair(usedOf(v, data.usage), v.limit, lang)}</span>
        </div>
      )}
      {N(r.note) && <div className="quote">{N(r.note)}</div>}
      {pend ? (
        <>
          <div className="acts">
            {r.type === 'limit' && (
              <div className="fieldw">
                <label htmlFor={`ra-${r.id}`} className="hint">{T('المبلغ المعتمد', 'Approved amount')} ({T('ج.س', 'SDG')})</label>
                <input className="field num" id={`ra-${r.id}`} type="number" min={10000} step={10000} value={amount} onChange={(e) => setAmount(e.target.value)} />
              </div>
            )}
            <div className="fieldw">
              <label htmlFor={`rn-${r.id}`} className="hint">{T('ملاحظة للسائق (اختياري)', 'Note to driver (optional)')}</label>
              <input className="field" id={`rn-${r.id}`} maxLength={140} value={note} onChange={(e) => setNote(e.target.value)} />
            </div>
          </div>
          <div className="acts">
            <button className="btn-ok" onClick={() => decide(true)}><Icon name="check" size={16} />{T('اعتماد', 'Approve')}</button>
            <button className="btn-no" onClick={() => decide(false)}><Icon name="x" size={16} />{T('رفض', 'Decline')}</button>
          </div>
        </>
      ) : (
        <div className="hint">
          {r.status === 'approved'
            ? r.type === 'limit' ? T(`اعتُمد ${curTxt(r.approved, lang)}`, `${curTxt(r.approved, lang)} approved`) : T('أُعيد التفعيل', 'Reactivated')
            : T('مرفوض', 'Declined')}
          {' · '}{whenLabel(r.decidedAt, lang)}
          {N(r.reply) ? ' · ' + N(r.reply) : ''}
        </div>
      )}
    </div>
  );
}

/** Tier budget card with its vehicles (prototype tierCard). */
export function TierCard({ t, data, onEdit }: { t: Tier; data: FleetData; onEdit?: () => void }) {
  const { T, N, lang } = useI18n();
  const s = tierStats(t, data.vehicles, data.usage);
  const driverName = useDriverName(data);
  return (
    <div className="card">
      <div className="tierhead">
        <div style={{ display: 'flex', gap: 10, minWidth: 0 }}>
          <span className={`ico ${t.color}`}><Icon name={t.icon} /></span>
          <div style={{ minWidth: 0 }}>
            <b className="w" style={{ fontFamily: 'var(--head)', fontSize: 16, display: 'block' }}>{N(t.name)}</b>
            <span className="muted" style={{ fontSize: 12 }}>
              <span className="num">{s.vs.length}</span> {T('مركبات', 'vehicles')} · {short(t.budget, lang)} {T('ج.س', 'SDG')} {T('لكل مركبة شهرياً', 'per vehicle / month')}
            </span>
          </div>
        </div>
        {onEdit && <button className="btn-sm b" onClick={onEdit}><Icon name="edit" size={15} />{T('تعديل', 'Edit')}</button>}
      </div>
      <div className="kv">
        <div><span>{T('إجمالي الميزانية', 'Total budget')}</span><b>{short(s.budget, lang)}</b></div>
        <div><span>{T('المصروف', 'Spent')}</span><b className={quotaCls(s.pct) || 'w'}>{short(s.spent, lang)}</b></div>
        <div><span>{T('المتبقي', 'Remaining')}</span><b className="em">{short(s.remain, lang)}</b></div>
      </div>
      <Quota used={s.spent} lim={s.budget} />
      <p className="hint" style={{ margin: '8px 0 0' }}>
        {T('الساعات', 'Hours')} <span className="num">{t.window}</span> · {T('الحد اليومي', 'Daily cap')} <span className="num">{t.dailyLitres}</span> {T('لتر', 'L')}
        {s.extra > 0 && <> · {T('زيادات معتمدة', 'Approved increases')} <span className="num em">{short(s.extra, lang)}</span></>}
      </p>
      <div style={{ marginTop: 10 }}>
        {s.vs.length ? s.vs.map((v) => {
          const d = v.driverId ? data.drivers.find((x) => x.id === v.driverId) : undefined;
          return (
            <div className="tierrow" key={v.id}>
              <span className="ava sm" style={{ background: `var(--${t.color})`, width: 32, height: 32, fontSize: 11 }}>{d ? initials(N(d.name), lang) : '—'}</span>
              <div className="t">
                <b>{driverName(v.driverId)}{d && !d.active && <> <span className="pill rd" style={{ fontSize: 10 }}>{T('موقوف', 'Suspended')}</span></>}</b>
                <span>{N(v.name)} · {plateTxt(v.plate, lang)}{v.customLimit != null ? ` · ${T('حد مخصص', 'custom limit')}` : ''}</span>
              </div>
              <div className="r"><span className="num">{pair(usedOf(v, data.usage), v.limit, lang)}</span><Quota used={usedOf(v, data.usage)} lim={v.limit} /></div>
            </div>
          );
        }) : <div className="empty" style={{ padding: 10 }}>{T('لا توجد مركبات في هذه الفئة.', 'No vehicles in this tier.')}</div>}
      </div>
    </div>
  );
}

type RuleKey = 'network' | 'wrongFuel' | 'odometer' | 'antiSiphon' | 'geofence' | 'maxFill' | 'hours';

/** The fuel rules with live toggles (prototype rulesBlock). */
export function RulesBlock({ rules, suffix, stationNames }: { rules: Rules; suffix: string; stationNames: string }) {
  const { T, lang } = useI18n();
  const run = useAction();
  const [maxAmt, setMaxAmt] = useState(String(rules.maxFillAmount));
  const toggle = (k: RuleKey, v: boolean) => void run(() => api.fleet.patchRules({ [k]: v }), T('تم تحديث القاعدة في جميع التطبيقات', 'Rule updated across all apps'));
  const row = (k: RuleKey, icon: string, c: string, title: string, sub: string, tag?: string) => (
    <div className="rule" key={k}>
      <span className={`ico ${c}`}><Icon name={icon} /></span>
      <div className="t"><b>{title}{tag && <> <span className="pill or" style={{ fontSize: 10 }}>{tag}</span></>}</b><span>{sub}</span></div>
      <Toggle id={`rule-${k}${suffix}`} checked={rules[k]} label={title} onChange={(v) => toggle(k, v)} />
    </div>
  );
  const saveMax = () => {
    const n = Math.max(10000, Number(maxAmt) || 400000);
    setMaxAmt(String(n));
    void run(() => api.fleet.patchRules({ maxFillAmount: n }), T(`الحد الأقصى للتعبئة الواحدة ${curTxt(n, lang)}`, `Maximum per fill-up set to ${curTxt(n, lang)}`));
  };
  return (
    <>
      {row('network', 'grid', 'or', T('التعبئة من محطات الشبكة فقط', 'Network stations only'), stationNames, T('صارم', 'Strict'))}
      {row('wrongFuel', 'nozzle', 'rd', T('منع نوع الوقود غير المطابق', 'Block mismatched fuel'), T('يُصرف نوع الوقود المسجّل للمركبة فقط', "Only the vehicle's registered grade is dispensed"))}
      {row('odometer', 'gauge', 'am', T('قراءة العدّاد قبل التعبئة', 'Odometer before fueling'), T('يبقى التصريح مقفلاً حتى يسجّل السائق القراءة', 'The pass stays locked until the driver records the reading'))}
      {row('antiSiphon', 'shield', 'vi', T('الحماية من السحب', 'Anti-siphoning'), T('تعبئة واحدة كحد أقصى كل 4 ساعات لكل مركبة', 'At most one fill-up every 4 hours per vehicle'))}
      {row('geofence', 'pin', 'cy', T('التحقق من الموقع', 'Location check'), T('يُقفل المسدس إذا كانت المركبة على بُعد أكثر من 50 متراً', 'Locks the nozzle if the vehicle is more than 50 m away'))}
      <div className="rule">
        <span className="ico bl"><Icon name="wallet" /></span>
        <div className="t"><b>{T('الحد الأقصى للتعبئة الواحدة', 'Maximum per fill-up')}</b><span>{T('بالجنيه السوداني', 'In Sudanese pounds')}</span></div>
        <input className="tinput" type="number" id={`maxamt${suffix}`} min={10000} step={10000} value={maxAmt} aria-label={T('المبلغ', 'Amount')}
          onChange={(e) => setMaxAmt(e.target.value)} onBlur={() => Number(maxAmt) !== rules.maxFillAmount && saveMax()}
          onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} />
        <Toggle id={`rule-maxFill${suffix}`} checked={rules.maxFill} label={T('الحد الأقصى للتعبئة الواحدة', 'Maximum per fill-up')} onChange={(v) => toggle('maxFill', v)} />
      </div>
      {row('hours', 'clock', 'em', T('ساعات التعبئة', 'Fueling hours'), T('حسب نافذة كل فئة', "Per each tier's time window"))}
    </>
  );
}

/** Station names of the network rule, joined for display. */
export function useNetworkNames(data: FleetData) {
  const { T, N } = useI18n();
  const names = (data.rules?.networkStationIds ?? []).map((id) => data.stations.find((s) => s.id === id)).filter(Boolean).map((s) => N(s!.name));
  return names.length ? names.join(T(' و', ' and ')) : '—';
}

/** Security log entry (prototype secLogCard). */
export function SecLogCard({ e }: { e: SecurityEvent }) {
  const { N, lang } = useI18n();
  const c = e.severity === 'critical' ? 'rd' : 'am';
  return (
    <div className="card" style={{ padding: 14 }}>
      <div className="alertc">
        <span className={`ico ${c}`}><Icon name={c === 'rd' ? 'siren' : 'alert'} /></span>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <b className={c} style={{ fontSize: 13 }}>{N(e.title)}</b>
            <span className="muted" style={{ fontSize: 12 }}>{agoLabel(e.createdAt, lang)}</span>
          </div>
          <p>{N(e.detail)}</p>
          <span className="pill num" style={{ marginTop: 6 }}>{e.ref}</span>
        </div>
      </div>
    </div>
  );
}

/** Computes alerts for the current fleet data. */
export function useAlerts(data: FleetData): Alert[] {
  const { T, N, lang } = useI18n();
  return fleetAlerts({ requests: data.requests, drivers: data.drivers, vehicles: data.vehicles, usage: data.usage }, lang, T, N);
}

/** Alert card; onReview is called for "review request" actions. */
export function AlertCard({ a, onReview }: { a: Alert; onReview: () => void }) {
  const { T, lang } = useI18n();
  const run = useAction();
  const toast = useToast();
  const act = () => {
    const x = a.action;
    switch (x.kind) {
      case 'review-requests': return onReview();
      case 'reactivate-driver': return void run(() => api.fleet.setDriverActive(x.id, true), T('تمت إعادة تفعيل السائق', 'Driver reactivated'));
      case 'reactivate-vehicle': return void run(() => api.fleet.setVehicleActive(x.id, true), T('تمت إعادة تفعيل المركبة', 'Vehicle reactivated'));
      case 'raise-limit': return void run(() => api.fleet.increaseLimit(x.id, x.amount),
        T(`تمت إضافة ${curTxt(x.amount, lang)} إلى حد هذا الشهر`, `${curTxt(x.amount, lang)} added to this month's limit`));
      case 'order-tag': return toast(T('تم طلب شريحة بديلة', 'Replacement tag ordered'));
    }
  };
  return (
    <div className="card" style={{ padding: 14 }}>
      <div className="alertc">
        <span className={`ico ${a.color}`}><Icon name={a.icon} /></span>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <b className="w">{a.title}</b><span className={`tag ${a.color}`}>{a.tag}</span>
          </div>
          <p>{a.detail}</p>
          <div style={{ marginTop: 8 }}><button className="link" onClick={act}>{a.actionLabel}</button></div>
        </div>
      </div>
    </div>
  );
}
