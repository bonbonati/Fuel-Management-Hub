/**
 * Page-level helpers of the web console (prototype: ph, wk, editBtn,
 * fleetTxTable) and the hash-based page state.
 */
import {
  FuelTag, Icon, PlateTag, StatusPill, sar, useI18n, vatOf, whenLabel,
  type Driver, type Station, type Tax, type Transaction, type Vehicle,
} from '@petro/shared';
import { useEffect, useState, type ReactNode } from 'react';

/** Page header with title, subtitle and actions. */
export function Ph({ h, p, acts }: { h: ReactNode; p?: ReactNode; acts?: ReactNode }) {
  return (
    <div className="ph">
      <div>
        <h1>{h}</h1>
        {p && <p>{p}</p>}
      </div>
      {acts && <div className="acts">{acts}</div>}
    </div>
  );
}

/** KPI card. */
export function Wk({ l, v, e, c, i }: { l: string; v: ReactNode; e: ReactNode; c: string; i: string }) {
  return (
    <div className="card">
      <div className="top">
        <span className="lbl">{l}</span>
        <span className={`ico ${c}`}><Icon name={i} size={17} /></span>
      </div>
      <b>{v}</b>
      <em>{e}</em>
    </div>
  );
}

/** Amount followed by the currency caption, for KPI cards. */
export function Amt({ n }: { n: number }) {
  const { T } = useI18n();
  return <>{sar(n)}<span className="cap">{T('ج.س', 'SDG')}</span></>;
}

export function EditBtn({ onClick }: { onClick: () => void }) {
  const { T } = useI18n();
  return (
    <button className="btn-sm b" onClick={onClick}>
      <Icon name="edit" size={15} />
      {T('تعديل', 'Edit')}
    </button>
  );
}

/** Fleet transaction table (prototype fleetTxTable). */
export function FleetTxTable({ rows, vehicles, drivers, stations, tax }: { rows: Transaction[]; vehicles: Vehicle[]; drivers: Driver[]; stations: Station[]; tax?: Tax }) {
  const { T, N, lang } = useI18n();
  if (!rows.length) return <div className="empty">{T('لا توجد عمليات مطابقة.', 'No matching transactions.')}</div>;
  const driverOf = (t: Transaction) => {
    const id = t.driverId || vehicles.find((v) => v.id === t.vehicleId)?.driverId;
    const d = drivers.find((x) => x.id === id);
    return d ? N(d.name) : T('غير مخصص', 'Unassigned');
  };
  return (
    <div className="tbl-wrap">
      <table>
        <thead>
          <tr>
            <th>{T('الإيصال', 'Receipt')}</th><th>{T('الوقت', 'Time')}</th><th>{T('المركبة', 'Vehicle')}</th><th>{T('السائق', 'Driver')}</th>
            <th>{T('المحطة', 'Station')}</th><th>{T('الوقود', 'Fuel')}</th><th>{T('اللترات', 'Litres')}</th><th>{T('المبلغ', 'Amount')}</th>
            {tax?.enabled && <th>{T('الضريبة', 'VAT')} {tax.rate}%</th>}
            <th>{T('الحالة', 'Status')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t) => (
            <tr key={t.id}>
              <td className="num">{t.id}</td>
              <td className="mut">{whenLabel(t.createdAt, lang)}</td>
              <td><PlateTag p={t.plate} /></td>
              <td>{driverOf(t)}</td>
              <td>{N(stations.find((s) => s.id === t.stationId)?.name) || t.stationId} · <span className="num">{t.pump}</span></td>
              <td><FuelTag f={t.fuel} /></td>
              <td className="num">{t.litres.toFixed(1)}</td>
              <td><b className="num">{sar(t.amount)}</b></td>
              {tax?.enabled && <td className="num vi">{sar(vatOf(t.amount, tax))}</td>}
              <td><StatusPill t={t} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * Page selection kept in location.hash ("#vehicles") so a reload or a shared
 * link opens the same page. Falls back to def for unknown values.
 */
export function useHashTab<T extends string>(valid: readonly T[], def: T): [T, (t: T) => void] {
  const read = () => {
    const h = window.location.hash.replace(/^#\/?/, '') as T;
    return valid.includes(h) ? h : def;
  };
  const [tab, setTab] = useState<T>(read);
  useEffect(() => {
    const on = () => setTab(read());
    window.addEventListener('hashchange', on);
    return () => window.removeEventListener('hashchange', on);
  });
  return [
    tab,
    (t: T) => {
      window.location.hash = t;
      setTab(t);
      window.scrollTo(0, 0);
    },
  ];
}
