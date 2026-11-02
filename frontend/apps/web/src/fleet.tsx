/**
 * Fleet portal pages (prototype wFleetOverview … wFleetAlerts). All data
 * comes from useFleetData(); every change goes through the API and is
 * reflected in the mobile apps on their next poll.
 */
import {
  AlertCard, api, curTxt, FUELFULL, FUELS, fleetTotals, FuelTag, Icon, monthName, pair, plus, PlateTag, Quota, ReqCardFleet,
  ReqFilterBar, RulesBlock, SecLogCard, short, TierCard, tierStats, Toggle, useAction, useAlerts, useDriverName, useI18n,
  useNetworkNames, usedOf, vatOf, weeklySpend, type FleetData, type ReqFilter, type Vehicle,
} from '@petro/shared';
import { useState } from 'react';
import { useOpenModal } from './modals';
import { Amt, EditBtn, FleetTxTable, Ph, Wk } from './ui';

/** Top-up amount of the "Top up" button (prototype: "SDG 5,000,000 added via Bankak"). */
export const TOP_UP = 5_000_000;

const fleetRows = (d: FleetData) => d.tx.filter((t) => t.vehicleId && d.vehicles.some((v) => v.id === t.vehicleId));

function TopUpBtn({ cls = 'wbtn' }: { cls?: string }) {
  const { T, lang } = useI18n();
  const run = useAction();
  return (
    <button className={cls} onClick={() => void run(() => api.payments.topUp(TOP_UP), T(`تم شحن ${curTxt(TOP_UP, lang)} عبر بنكك`, `${curTxt(TOP_UP, lang)} added via Bankak`))}>
      <Icon name="plus" />{T('شحن الرصيد', 'Top up')}
    </button>
  );
}

export function FleetOverview({ d, go }: { d: FleetData; go: (t: string) => void }) {
  const { T, N, lang } = useI18n();
  const { used, pct, litres } = fleetTotals(d.vehicles, d.usage, d.prices);
  const w = weeklySpend(used), mx = Math.max(1, ...w);
  const pr = d.requests.filter((r) => r.status === 'pending');
  const month = d.wallet ? monthName(d.wallet.month, lang) : '';
  const barC = ['#2F6BFF', '#7C5CFF', '#0891B2', '#00A86B'];
  return (
    <>
      <Ph h={T('نظرة عامة على الأسطول', 'Fleet overview')} p={T(`${month} · تحديث مباشر من المضخات`, `${month} · live from station pumps`)}
        acts={<><TopUpBtn /><button className="wbtn s" onClick={() => go('tx')}><Icon name="download" />{T('تصدير', 'Export')}</button></>} />
      <div className="wk">
        <Wk l={T('الرصيد المسبق', 'Prepaid balance')} v={<Amt n={d.wallet?.balance ?? 0} />} e={T('متاح للتعبئة', 'available for fueling')} c="bl" i="wallet" />
        <Wk l={T('مصروف الشهر', 'Month spend')} v={<Amt n={used} />} e={T(`${pct.toFixed(0)}% من إجمالي الحدود`, `${pct.toFixed(0)}% of total limits`)} c="vi" i="chart" />
        <Wk l={T('طلبات بانتظار الاعتماد', 'Requests awaiting approval')} v={pr.length} e={T('من السائقين', 'from drivers')} c="or" i="inbox" />
        <Wk l={T('اللترات', 'Litres')} v={litres.toFixed(0)} e={T('هذا الشهر', 'this month')} c="cy" i="nozzle" />
      </div>
      <div className="grid2">
        <div className="card panel">
          <h2>{T('ميزانيات الفئات', 'Tier budgets')}<button className="btn-sm" onClick={() => go('tiers')}>{T('إدارة الفئات', 'Manage tiers')}</button></h2>
          {d.tiers.map((t) => {
            const s = tierStats(t, d.vehicles, d.usage);
            return (
              <div className="li" key={t.id}>
                <span className={`ico ${t.color}`}><Icon name={t.icon} /></span>
                <div className="t">
                  <b>{N(t.name)}</b>
                  <span><span className="num">{s.vs.length}</span> {T('مركبات', 'vehicles')} · {T('المصروف', 'spent')} {short(s.spent, lang)} {T('من', 'of')} {short(s.budget, lang)}</span>
                  <Quota used={s.spent} lim={s.budget} />
                </div>
                <div className="r"><b className="num em">{short(s.remain, lang)}</b><span className="hint">{T('متبقٍ', 'left')}</span></div>
              </div>
            );
          })}
        </div>
        <div className="card panel">
          <h2>{T('المصروف الأسبوعي', 'Weekly spend')}<span>{T('ج.س', 'SDG')}</span></h2>
          <div className="chart" style={{ height: 170 }}>
            {w.map((v, i) => (
              <div key={i}><span className="num w">{short(v, lang)}</span><i style={{ height: `${(v / mx) * 74}%`, background: barC[i] }} /><span>{T('الأسبوع', 'Week')} {i + 1}</span></div>
            ))}
          </div>
        </div>
      </div>
      {pr.length > 0 && (
        <div className="card panel">
          <h2>{T('طلبات بانتظار الاعتماد', 'Requests awaiting approval')}<button className="btn-sm" onClick={() => go('requests')}>{T('عرض الكل', 'View all')}</button></h2>
          <div className="reqgrid">{pr.slice(0, 3).map((r) => <ReqCardFleet key={r.id} r={r} data={d} />)}</div>
        </div>
      )}
      <div className="card panel">
        <h2>{T('آخر التعبئات', 'Latest fill-ups')}<button className="btn-sm" onClick={() => go('tx')}>{T('عرض الكل', 'View all')}</button></h2>
        <FleetTxTable rows={fleetRows(d).slice(0, 5)} vehicles={d.vehicles} drivers={d.drivers} stations={d.stations} tax={d.tax} />
      </div>
    </>
  );
}

export function FleetRequests({ d }: { d: FleetData }) {
  const { T } = useI18n();
  const [f, setF] = useState<ReqFilter>('pending');
  const rs = d.requests.filter((r) => f === 'all' || r.status === f);
  return (
    <>
      <Ph h={T('طلبات السائقين', 'Driver requests')} p={T('راجع طلبات زيادة الحد وإعادة التفعيل. تُضاف الزيادة المعتمدة إلى حد المركبة لهذا الشهر وتُحتسب ضمن ميزانية فئتها.',
        "Review limit increase and reactivation requests. An approved increase is added to the vehicle's limit for this month and counted in its tier budget.")} />
      <div className="card panel">
        <ReqFilterBar value={f} onChange={setF} requests={d.requests} />
        <div className="reqgrid">{rs.length ? rs.map((r) => <ReqCardFleet key={r.id} r={r} data={d} />) : <div className="empty">{T('لا توجد طلبات في هذه القائمة.', 'No requests in this list.')}</div>}</div>
      </div>
    </>
  );
}

function VehicleRow({ v, d }: { v: Vehicle; d: FleetData }) {
  const { T, N, lang } = useI18n();
  const run = useAction();
  const open = useOpenModal();
  const driverName = useDriverName(d);
  const used = usedOf(v, d.usage);
  const setTier = (tierId: string) => {
    const t = d.tiers.find((x) => x.id === tierId);
    void run(() => api.fleet.setVehicleTier(v.id, tierId || null), (res) =>
      t ? T(`نُقلت المركبة إلى فئة ${t.name.ar} · الحد ${curTxt(res.limit, lang)}`, `Vehicle moved to ${t.name.en} · limit ${curTxt(res.limit, lang)}`)
        : T('أُزيلت المركبة من الفئة', 'Vehicle removed from its tier'));
  };
  return (
    <tr className={v.active ? '' : 'off'}>
      <td><PlateTag p={v.plate} /></td>
      <td><b>{N(v.name)}</b></td>
      <td>{driverName(v.driverId)}</td>
      <td>
        <select className="tinput" style={{ fontFamily: 'var(--body)', width: 'auto' }} aria-label={T('الفئة', 'Tier')} value={v.tierId ?? ''} onChange={(e) => setTier(e.target.value)}>
          <option value="">{T('بدون فئة', 'No tier')}</option>
          {d.tiers.map((t) => <option key={t.id} value={t.id}>{N(t.name)}</option>)}
        </select>
      </td>
      <td><FuelTag f={v.fuel} /></td>
      <td>{v.tagBattery < 15 ? <span className="pill am"><Icon name="battery" size={13} /><span className="num">{v.tagBattery}%</span></span> : <span className="pill em"><span className="num">{v.tagBattery}%</span></span>}</td>
      <td className="mini">
        <span className="num" style={{ fontSize: 12 }}>{pair(used, v.limit, lang)}</span>
        <Quota used={used} lim={v.limit} />
        {(v.customLimit != null || v.extra > 0) && (
          <span className="hint">{v.customLimit != null ? T('حد مخصص', 'Custom') : ''}{v.customLimit != null && v.extra ? ' · ' : ''}{v.extra ? plus(v.extra, lang) : ''}</span>
        )}
      </td>
      <td>
        <Toggle id={`act-${v.id}`} checked={v.active} label={T('تفعيل المركبة', 'Vehicle active')} onChange={(on) => void run(() => api.fleet.setVehicleActive(v.id, on),
          on ? T('تمت إعادة تفعيل المركبة', 'Vehicle reactivated') : T('تم إيقاف المركبة ولن يتمكن سائقها من التعبئة', 'Vehicle suspended; its driver can no longer fuel'))} />
      </td>
      <td><EditBtn onClick={() => open({ type: 'vehicle', id: v.id })} /></td>
    </tr>
  );
}

export function FleetVehicles({ d }: { d: FleetData }) {
  const { T } = useI18n();
  const open = useOpenModal();
  return (
    <>
      <Ph h={T('المركبات', 'Vehicles')} p={T('يُحدَّد الحد الشهري لكل مركبة من ميزانية فئتها، ويمكن تخصيص حد مختلف لمركبة بعينها.', "Each vehicle's monthly limit comes from its tier budget. You can set a custom limit for a specific vehicle.")}
        acts={<button className="wbtn" onClick={() => open({ type: 'vehicle', id: 'new' })}><Icon name="plus" />{T('إضافة مركبة', 'Add vehicle')}</button>} />
      <div className="card panel">
        <div className="tbl-wrap">
          <table>
            <thead><tr>
              <th>{T('اللوحة', 'Plate')}</th><th>{T('الطراز', 'Model')}</th><th>{T('السائق', 'Driver')}</th><th>{T('الفئة', 'Tier')}</th><th>{T('الوقود', 'Fuel')}</th>
              <th>RFID</th><th>{T('المصروف / الحد', 'Spent / limit')}</th><th>{T('نشطة', 'Active')}</th><th />
            </tr></thead>
            <tbody>{d.vehicles.map((v) => <VehicleRow key={v.id} v={v} d={d} />)}</tbody>
          </table>
        </div>
      </div>
    </>
  );
}

export function FleetDrivers({ d }: { d: FleetData }) {
  const { T, N, lang } = useI18n();
  const run = useAction();
  const open = useOpenModal();
  return (
    <>
      <Ph h={T('السائقون', 'Drivers')} p={T('أضف السائقين وعدّل بياناتهم أو أوقف حساباتهم. لا يستطيع السائق الموقوف استخدام تصريح الوقود.', 'Add drivers, edit their details or suspend their accounts. A suspended driver cannot use the fuel pass.')}
        acts={<button className="wbtn" onClick={() => open({ type: 'driver', id: 'new' })}><Icon name="plus" />{T('إضافة سائق', 'Add driver')}</button>} />
      <div className="card panel">
        <div className="tbl-wrap">
          <table>
            <thead><tr>
              <th>{T('السائق', 'Driver')}</th><th>{T('الرقم', 'ID')}</th><th>{T('الجوال', 'Mobile')}</th><th>{T('المركبة', 'Vehicle')}</th><th>{T('الفئة', 'Tier')}</th>
              <th>{T('الحالة', 'Status')}</th><th>{T('المصروف / الحد', 'Spent / limit')}</th><th>{T('مفعّل', 'Active')}</th><th />
            </tr></thead>
            <tbody>
              {d.drivers.map((dr) => {
                const v = d.vehicles.find((x) => x.driverId === dr.id);
                const t = v ? d.tiers.find((x) => x.id === v.tierId) : undefined;
                return (
                  <tr key={dr.id} className={dr.active ? '' : 'off'}>
                    <td><b>{N(dr.name)}</b></td>
                    <td className="num mut">{dr.code}</td>
                    <td className="num ltr mut">{dr.phone}</td>
                    <td>{v ? <PlateTag p={v.plate} /> : '—'}</td>
                    <td>{t ? <span className={`pill ${t.color}`}>{N(t.name)}</span> : '—'}</td>
                    <td>{!dr.active ? <span className="pill rd">{T('موقوف', 'Suspended')}</span> : dr.appActivated
                      ? <span className="pill em"><span className="dot" />{T('التطبيق مفعّل', 'App active')}</span> : <span className="pill am">{T('بانتظار التفعيل', 'Invite pending')}</span>}</td>
                    <td className="mini">{v ? <><span className="num" style={{ fontSize: 12 }}>{pair(usedOf(v, d.usage), v.limit, lang)}</span><Quota used={usedOf(v, d.usage)} lim={v.limit} /></> : '—'}</td>
                    <td>
                      <Toggle id={`den-${dr.id}`} checked={dr.active} label={`${T('تفعيل السائق', 'Driver active')} ${N(dr.name)}`} onChange={(on) => void run(() => api.fleet.setDriverActive(dr.id, on),
                        on ? T(`تمت إعادة تفعيل ${dr.name.ar}`, `${dr.name.en} reactivated`) : T(`تم إيقاف ${dr.name.ar} وإقفال تصريح الوقود`, `${dr.name.en} suspended and fuel pass locked`))} />
                    </td>
                    <td><EditBtn onClick={() => open({ type: 'driver', id: dr.id })} /></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

export function FleetTiers({ d }: { d: FleetData }) {
  const { T } = useI18n();
  const open = useOpenModal();
  const tot = d.tiers.reduce((a, t) => { const s = tierStats(t, d.vehicles, d.usage); a.b += s.budget; a.s += s.spent; return a; }, { b: 0, s: 0 });
  const pct = ((tot.s / Math.max(1, tot.b)) * 100).toFixed(0);
  return (
    <>
      <Ph h={T('الفئات والميزانيات', 'Tiers & budgets')} p={T('اربط المركبات وسائقيها بالفئة المناسبة. ميزانية الفئة = ميزانية المركبة × عدد المركبات، ويُخصم منها كل ما يصرفه سائقوها.',
        'Assign vehicles and their drivers to a tier. Tier budget = per-vehicle budget × number of vehicles, and every fill-up by its drivers is deducted from it.')}
        acts={<button className="wbtn" onClick={() => open({ type: 'tier', id: 'new' })}><Icon name="plus" />{T('فئة جديدة', 'New tier')}</button>} />
      <div className="wk">
        <Wk l={T('إجمالي الميزانيات', 'Total budgets')} v={<Amt n={tot.b} />} e={T('لهذا الشهر', 'this month')} c="bl" i="layers" />
        <Wk l={T('المصروف', 'Spent')} v={<Amt n={tot.s} />} e={T(`${pct}% من الميزانيات`, `${pct}% of budgets`)} c="vi" i="chart" />
        <Wk l={T('المتبقي', 'Remaining')} v={<Amt n={Math.max(0, tot.b - tot.s)} />} e={T('قابل للصرف', 'available to spend')} c="em" i="wallet" />
        <Wk l={T('مركبات بلا فئة', 'Vehicles without a tier')} v={d.vehicles.filter((v) => !d.tiers.some((t) => t.id === v.tierId)).length} e={T('يجب ربطها بفئة', 'should be assigned')} c="or" i="alert" />
      </div>
      <div className="reqgrid" style={{ gridTemplateColumns: 'repeat(auto-fill,minmax(340px,1fr))' }}>
        {d.tiers.map((t) => <TierCard key={t.id} t={t} data={d} onEdit={() => open({ type: 'tier', id: t.id })} />)}
      </div>
    </>
  );
}

export function FleetRules({ d }: { d: FleetData }) {
  const { T } = useI18n();
  const names = useNetworkNames(d);
  return (
    <>
      <Ph h={T('قواعد الوقود ومكافحة الاحتيال', 'Fuel rules & fraud controls')} p={T('تُطبَّق عند المضخة وفي تطبيق السائق وماسح العامل.', 'Enforced at the pump, in the driver app and on the attendant scanner.')} />
      <div className="grid2">
        <div className="card panel"><h2>{T('القواعد العامة', 'General rules')}</h2>{d.rules && <RulesBlock key={d.rules.maxFillAmount} rules={d.rules} suffix="-w" stationNames={names} />}</div>
        <div className="card panel"><h2>{T('سجل الأمان', 'Security log')}</h2><div className="stack">{d.security.map((e) => <SecLogCard key={e.id} e={e} />)}</div></div>
      </div>
    </>
  );
}

export function FleetTx({ d, go }: { d: FleetData; go: (t: string) => void }) {
  const { T, N } = useI18n();
  const [q, setQ] = useState('');
  const [fuel, setFuel] = useState('all');
  const done = fleetRows(d).filter((t) => t.status === 'done');
  const tot = done.reduce((a, t) => a + t.amount, 0);
  const query = q.trim().toLowerCase();
  const rows = fleetRows(d).filter((t) => {
    if (fuel !== 'all' && t.fuel !== fuel) return false;
    if (!query) return true;
    const v = d.vehicles.find((x) => x.id === t.vehicleId);
    const dr = d.drivers.find((x) => x.id === (t.driverId || v?.driverId));
    return [t.id, t.plate.digits, t.plate.latin, dr ? dr.name.ar + ' ' + dr.name.en : ''].join(' ').toLowerCase().includes(query);
  });
  return (
    <>
      <Ph h={T('العمليات', 'Transactions')} p={T('جميع تعبئات مركبات الشركة مع إيصالاتها الإلكترونية.', 'All company fill-ups with their e-receipts.')}
        acts={<>
          <button className="wbtn s" onClick={() => go('settings')}><Icon name="percent" />{d.tax?.enabled ? T(`الضريبة ${d.tax.rate}%`, `VAT ${d.tax.rate}%`) : T('الضريبة غير مفعّلة', 'VAT off')}</button>
          <DownloadBtn label={T('تنزيل الكشف', 'Download statement')} done={T('تم تنزيل الكشف', 'Statement downloaded')} />
        </>} />
      <div className="wk">
        <Wk l={T('المصروف', 'Spent')} v={<Amt n={tot} />} e={T('عمليات مكتملة', 'completed fill-ups')} c="bl" i="wallet" />
        {d.tax?.enabled
          ? <Wk l={T('ضريبة المدخلات', 'Input VAT')} v={<Amt n={vatOf(tot, d.tax)} />} e={`${d.tax.rate}%`} c="vi" i="percent" />
          : <Wk l={T('الضريبة', 'VAT')} v={T('غير مفعّلة', 'Off')} e={T('من الإعدادات', 'in Settings')} c="vi" i="percent" />}
        <Wk l={T('اللترات', 'Litres')} v={done.reduce((a, t) => a + t.litres, 0).toFixed(0)} e={T('هذا الشهر', 'this month')} c="cy" i="nozzle" />
        <Wk l={T('الإيصالات', 'Receipts')} v={done.length} e={T('إيصالات إلكترونية', 'e-receipts')} c="em" i="receipt" />
      </div>
      <div className="card panel">
        <div className="filters">
          <input type="search" aria-label={T('بحث', 'Search')} placeholder={T('ابحث برقم الإيصال أو اللوحة أو اسم السائق', 'Search by receipt, plate or driver')} value={q} onChange={(e) => setQ(e.target.value)} />
          <select aria-label={T('الوقود', 'Fuel')} value={fuel} onChange={(e) => setFuel(e.target.value)}>
            <option value="all">{T('جميع أنواع الوقود', 'All fuel types')}</option>
            {FUELS.map((k) => <option key={k} value={k}>{N(FUELFULL[k])}</option>)}
          </select>
        </div>
        <FleetTxTable rows={rows} vehicles={d.vehicles} drivers={d.drivers} stations={d.stations} tax={d.tax} />
      </div>
    </>
  );
}

/** Demo download button (statements/exports are not generated yet). */
export function DownloadBtn({ label, done }: { label: string; done: string }) {
  const run = useAction();
  return <button className="wbtn s" onClick={() => void run(async () => undefined, done)}><Icon name="download" />{label}</button>;
}

export function FleetAlerts({ d, go }: { d: FleetData; go: (t: string) => void }) {
  const { T } = useI18n();
  const alerts = useAlerts(d);
  return (
    <>
      <Ph h={T('التنبيهات', 'Alerts')} p={T('تصل أيضاً كإشعارات إلى تطبيق مدير الأسطول.', "Also sent as notifications to the fleet manager's app.")} />
      <div className="reqgrid">
        {alerts.length ? alerts.map((a, i) => <AlertCard key={i} a={a} onReview={() => go('requests')} />) : <div className="empty">{T('لا توجد تنبيهات.', 'No alerts.')}</div>}
      </div>
    </>
  );
}

export { TopUpBtn };
