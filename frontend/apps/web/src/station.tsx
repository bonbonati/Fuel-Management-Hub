/**
 * Station portal pages (prototype wStOverview … wStSettle) and the settings
 * page shared by both portals (wSettings).
 */
import {
  api, dateLabel, FUELS, FuelTag, Icon, Money, PlateTag, plateTxt, pumpState, roleName, sar, shiftName, shiftStats,
  StatusPill, Toggle, useAction, useI18n, useOrg, useSettlements, useStaff, useTax, useToast, useTxActions, vatOf, whenLabel,
  type Pump, type StationData, type Tax,
} from '@petro/shared';
import { useState } from 'react';
import { DownloadBtn } from './fleet';
import { useOpenModal } from './modals';
import { Amt, EditBtn, Ph, Wk } from './ui';

export const pumpLabel = (s: 'off' | 'busy' | 'free', T: (a: string, e: string) => string) =>
  s === 'off' ? T('متوقفة', 'Offline') : s === 'busy' ? T('قيد الاستخدام', 'In use') : T('متاحة', 'Available');

export function StOverview({ s }: { s: StationData }) {
  const { T, N, lang } = useI18n();
  const { start, finish } = useTxActions();
  const st = shiftStats(s.tx);
  const pend = s.tx.filter((t) => t.status !== 'done');
  const mix = FUELS.map((k) => ({ k, v: st.d.filter((t) => t.fuel === k).reduce((a, t) => a + t.amount, 0) }));
  const mm = Math.max(1, ...mix.map((m) => m.v));
  return (
    <>
      <Ph h={N(s.station?.name)} p={T('لوحة المشرف · اليوم', 'Supervisor console · today')} />
      <div className="wk">
        <Wk l={T('مبيعات اليوم', "Today's sales")} v={<Amt n={st.sales} />} e={s.tax?.enabled ? `${T('الضريبة', 'VAT')} ${sar(vatOf(st.sales, s.tax))}` : T('بدون ضريبة', 'No VAT')} c="or" i="wallet" />
        <Wk l={T('العمليات', 'Fill-ups')} v={st.d.length} e={<><span className="num">{st.lit.toFixed(0)}</span> {T('لتر', 'litres')}</>} c="cy" i="nozzle" />
        <Wk l={T('قيد التنفيذ', 'In progress')} v={pend.length} e={T('من التطبيق والماسح', 'from app and scanner')} c="am" i="clock" />
        <Wk l={T('المضخات العاملة', 'Pumps online')} v={`${s.pumps.filter((p) => p.online).length}/${s.pumps.length}`} e={T('من صفحة المضخات', 'managed in Pumps')} c="em" i="pump" />
      </div>
      <div className="grid2">
        <div className="card panel">
          <h2>{T('العمليات الجارية', 'Live operations')}<span>{T('تظهر أيضاً في تطبيق العامل', 'also on the attendant app')}</span></h2>
          {pend.length ? pend.map((t) => (
            <div className="li" key={t.id}>
              <span className="ico or"><b className="num">{t.pump}</b></span>
              <div className="t">
                <b><Money n={t.amount} /> · <FuelTag f={t.fuel} /></b>
                <span>{plateTxt(t.plate, lang)} · {t.code ? <>{T('الرمز', 'code')} <span className="num">{t.code}</span> · </> : ''}{whenLabel(t.createdAt, lang)}</span>
              </div>
              <div className="r">
                <StatusPill t={t} />
                {t.status === 'auth'
                  ? <button className="btn-sm p" onClick={() => void start(t)}>{T('تشغيل', 'Start')}</button>
                  : <button className="btn-sm p" onClick={() => void finish(t)}>{T('إنهاء', 'Finish')}</button>}
              </div>
            </div>
          )) : <div className="empty">{T('لا توجد عمليات جارية.', 'No live operations.')}</div>}
        </div>
        <div className="card panel">
          <h2>{T('المبيعات حسب نوع الوقود', 'Sales by fuel')}</h2>
          {mix.map((m) => (
            <div className="hbar" key={m.k}>
              <span className="l"><FuelTag f={m.k} /></span>
              <span className="track"><i style={{ width: `${(m.v / mm) * 100}%`, background: `var(--${m.k === '91' ? 'g91' : m.k === '95' ? 'g95' : 'am-f'})` }} /></span>
              <span className="v"><Money n={m.v} /></span>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}

function PumpCard({ p, s }: { p: Pump; s: StationData }) {
  const { T, lang } = useI18n();
  const run = useAction();
  const open = useOpenModal();
  const st = pumpState(p.online, p.number, s.tx);
  const t = s.tx.find((x) => x.pump === p.number && x.status !== 'done');
  return (
    <div className={`card pumpc ${st === 'off' ? 'off' : ''}`}>
      <div className="hd">
        <span className="n">{String(p.number).padStart(2, '0')}</span>
        <Toggle id={`pump-${p.number}`} checked={p.online} label={`${T('تشغيل المضخة', 'Pump online')} ${p.number}`} onChange={(on) => void run(() => api.stations.setPumpOnline(p.stationId, p.number, on),
          on ? T(`المضخة ${p.number} تعمل الآن`, `Pump ${p.number} is back online`) : T(`تم إيقاف المضخة ${p.number}`, `Pump ${p.number} taken offline`))} />
      </div>
      <span className={`pill ${st === 'busy' ? 'cy' : st === 'off' ? 'rd' : 'em'}`}><span className="dot" />{pumpLabel(st, T)}</span>
      <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>{p.grades.map((g) => <FuelTag key={g} f={g} />)}</div>
      <span className="muted" style={{ fontSize: 12 }}>{t ? `${sar(t.amount)} ${T('ج.س', 'SDG')} · ${plateTxt(t.plate, lang)}` : <><span className="num">{p.flowRate}</span> {T('لتر/دقيقة', 'L/min')}</>}</span>
      <EditBtn onClick={() => open({ type: 'pump', id: String(p.number) })} />
    </div>
  );
}

export function StPumps({ s }: { s: StationData }) {
  const { T } = useI18n();
  return (
    <>
      <Ph h={T('المضخات', 'Pumps')} p={T('حدّد أنواع الوقود ومعدل التدفق لكل مضخة، أو أوقفها للصيانة.', "Set each pump's fuel grades and flow rate, or take it offline for maintenance.")} />
      <div className="pumps">{s.pumps.map((p) => <PumpCard key={p.number} p={p} s={s} />)}</div>
    </>
  );
}

export function StTx({ s, go }: { s: StationData; go: (t: string) => void }) {
  const { T, lang } = useI18n();
  return (
    <>
      <Ph h={T('عمليات المحطة', 'Station transactions')} p={T('مدفوعات تطبيق بترو والماسح في هذه المحطة.', 'Petro app and scanner payments at this station.')}
        acts={<>
          <button className="wbtn s" onClick={() => go('settings')}><Icon name="percent" />{s.tax?.enabled ? T(`الضريبة ${s.tax.rate}%`, `VAT ${s.tax.rate}%`) : T('الضريبة غير مفعّلة', 'VAT off')}</button>
          <DownloadBtn label={T('تصدير', 'Export')} done={T('تم تصدير الملف', 'Export ready')} />
        </>} />
      <div className="card panel">
        <div className="tbl-wrap">
          <table>
            <thead><tr>
              <th>{T('العملية', 'Transaction')}</th><th>{T('الوقت', 'Time')}</th><th>{T('المضخة', 'Pump')}</th><th>{T('المركبة', 'Vehicle')}</th><th>{T('العميل', 'Customer')}</th>
              <th>{T('الطريقة', 'Method')}</th><th>{T('الوقود', 'Fuel')}</th><th>{T('المبلغ', 'Amount')}</th>{s.tax?.enabled && <th>{T('الضريبة', 'VAT')}</th>}<th>{T('الحالة', 'Status')}</th>
            </tr></thead>
            <tbody>
              {s.tx.map((t) => (
                <tr key={t.id}>
                  <td className="num">{t.id}</td>
                  <td className="mut">{whenLabel(t.createdAt, lang)}</td>
                  <td className="num">{t.pump}</td>
                  <td><PlateTag p={t.plate} /></td>
                  <td>{t.orgId ? T('حساب أسطول', 'Fleet account') : T('عميل فرد', 'Individual')}</td>
                  <td>{t.source === 'qr' ? <span className="pill vi">QR</span> : t.source === 'app' ? <span className="pill em">{T('التطبيق', 'App')}</span> : <span className="pill cy">RFID</span>}</td>
                  <td><FuelTag f={t.fuel} /></td>
                  <td><b className="num">{sar(t.amount)}</b></td>
                  {s.tax?.enabled && <td className="num vi">{sar(vatOf(t.amount, s.tax))}</td>}
                  <td><StatusPill t={t} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

export function StStaff({ s }: { s: StationData }) {
  const { T, N } = useI18n();
  const run = useAction();
  const open = useOpenModal();
  const staff = useStaff(s.stationId).data ?? [];
  return (
    <>
      <Ph h={T('الموظفون والورديات', 'Staff & shifts')} p={T('أضف العاملين وحدّد أدوارهم وورديّاتهم أو أوقف حساباتهم.', 'Add attendants, set their roles and shifts, or suspend their accounts.')}
        acts={<button className="wbtn" onClick={() => open({ type: 'staff', id: 'new' })}><Icon name="plus" />{T('إضافة موظف', 'Add staff member')}</button>} />
      <div className="card panel">
        <div className="tbl-wrap">
          <table>
            <thead><tr><th>{T('الاسم', 'Name')}</th><th>{T('الرقم الوظيفي', 'Staff ID')}</th><th>{T('الدور', 'Role')}</th><th>{T('الوردية', 'Shift')}</th><th>{T('الجهاز', 'Device')}</th><th>{T('مفعّل', 'Active')}</th><th /></tr></thead>
            <tbody>
              {staff.map((m) => (
                <tr key={m.id} className={m.active ? '' : 'off'}>
                  <td><b>{N(m.name)}</b></td>
                  <td className="num mut">{m.code}</td>
                  <td>{N(roleName(m.role))}</td>
                  <td className="num">{shiftName(m.shift)}</td>
                  <td className="mut">{m.device}</td>
                  <td>
                    <Toggle id={`son-${m.id}`} checked={m.active} label={T('تفعيل الموظف', 'Staff active')} onChange={(on) => void run(() => api.stations.setStaffActive(m.stationId, m.id, on),
                      on ? T('تم تفعيل حساب الموظف', 'Staff account activated') : T('تم إيقاف حساب الموظف', 'Staff account suspended'))} />
                  </td>
                  <td><EditBtn onClick={() => open({ type: 'staff', id: m.id })} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

export function StSettle({ s }: { s: StationData }) {
  const { T, lang } = useI18n();
  const sum = useSettlements().data;
  const mask = s.station?.bankMask ? ` •• ${s.station.bankMask}` : '';
  return (
    <>
      <Ph h={T('التسويات', 'Settlements')} p={T(`تحويلات يومية إلى الحساب البنكي للمحطة${mask}`, `Daily transfers to the station's bank account${mask}`)} />
      <div className="wk">
        <Wk l={T('التحويل القادم', 'Next payout')} v={<Amt n={sum?.nextPayout?.amount ?? 0} />} e={sum?.nextPayout ? dateLabel(sum.nextPayout.date, lang) : '—'} c="em" i="bank" />
        <Wk l={T('رسوم الشهر', 'Fees this month')} v={<Amt n={sum?.feesThisMonth ?? 0} />} e={T('رسوم المنصة', 'platform fee')} c="or" i="receipt" />
      </div>
      <div className="card panel">
        <div className="tbl-wrap">
          <table>
            <thead><tr><th>{T('التاريخ', 'Date')}</th><th>{T('المبلغ', 'Amount')}</th><th>{T('الحالة', 'Status')}</th></tr></thead>
            <tbody>
              {(sum?.items ?? []).map((p) => (
                <tr key={p.date}>
                  <td>{dateLabel(p.date, lang)}</td>
                  <td><Money n={p.amount} /></td>
                  <td>{p.status === 'paid' ? <span className="pill em">{T('تم التحويل', 'Paid out')}</span> : <span className="pill am">{T('مجدول', 'Scheduled')}</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

/** Text field saved on blur (prototype: data-set change handler). */
function SettingField({ id, label, value, save, ltr, mono }: { id: string; label: string; value: string; save: (v: string) => Promise<unknown>; ltr?: boolean; mono?: boolean }) {
  const { T } = useI18n();
  const run = useAction();
  const toast = useToast();
  const [v, setV] = useState(value);
  const commit = () => {
    const val = v.trim();
    if (val === value) return;
    if (!val) {
      setV(value);
      return toast(T('لا يمكن ترك هذا الحقل فارغاً', 'This field cannot be empty'));
    }
    void run(() => save(val), T('تم الحفظ', 'Saved'));
  };
  return (
    <div className="fieldw">
      <label htmlFor={id}>{label}</label>
      <input className={`field${mono ? ' num' : ''}`} id={id} value={v} dir={ltr ? 'ltr' : undefined}
        onChange={(e) => setV(e.target.value)} onBlur={commit} onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} />
      {!v.trim() && <small className="hint rd">{T('لا يمكن ترك هذا الحقل فارغاً', 'This field cannot be empty')}</small>}
    </div>
  );
}

function VatCard() {
  const { T } = useI18n();
  const run = useAction();
  const toast = useToast();
  const tax: Tax | undefined = useTax().data;
  const [rate, setRate] = useState<string | null>(null);
  if (!tax) return null;
  const shown = rate ?? String(tax.rate);
  const saveRate = () => {
    const r = Number(shown);
    if (shown === '' || !(r >= 0 && r <= 100)) {
      setRate(null);
      return toast(T('أدخل نسبة بين 0 و 100', 'Enter a rate between 0 and 100'));
    }
    if (r !== tax.rate) void run(() => api.payments.setTax({ enabled: tax.enabled, rate: r }), T(`نسبة الضريبة ${r}%`, `VAT rate set to ${r}%`)).then(() => setRate(null));
  };
  return (
    <div className="card panel">
      <h2>{T('ضريبة القيمة المضافة', 'Value added tax (VAT)')}</h2>
      <div className="rule">
        <span className="ico vi"><Icon name="percent" /></span>
        <div className="t"><b>{T('تطبيق الضريبة', 'Apply VAT')}</b><span>{T('عند التفعيل تظهر الضريبة في الإيصالات والتقارير.', 'When on, VAT appears on receipts and reports.')}</span></div>
        <Toggle id="vat-on" checked={tax.enabled} label={T('تطبيق الضريبة', 'Apply VAT')} onChange={(on) => void run(() => api.payments.setTax({ enabled: on, rate: tax.rate }),
          on ? T(`تم تفعيل الضريبة بنسبة ${tax.rate}%`, `VAT turned on at ${tax.rate}%`) : T('تم إيقاف الضريبة', 'VAT turned off'))} />
      </div>
      {tax.enabled ? (
        <div className="fieldw" style={{ marginTop: 14 }}>
          <label htmlFor="vat-rate">{T('نسبة الضريبة (%)', 'VAT rate (%)')}</label>
          <input className="field num" id="vat-rate" type="number" min={0} max={100} step={0.5} value={shown} style={{ maxWidth: 180 }}
            onChange={(e) => setRate(e.target.value)} onBlur={saveRate} onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} />
          <span className="hint">{T('مثال: على مبلغ 100,000 ج.س شامل الضريبة، قيمة الضريبة', 'Example: on SDG 100,000 including VAT, the VAT is')} <b className="num vi">{sar(vatOf(100000, tax))} {T('ج.س', 'SDG')}</b></span>
        </div>
      ) : (
        <div className="note am" style={{ marginTop: 14, fontWeight: 500 }}><Icon name="info" size={16} />{T('الضريبة غير مفعّلة. تصدر الإيصالات بدون بند ضريبة.', 'VAT is off. Receipts are issued without a VAT line.')}</div>
      )}
    </div>
  );
}

export function Settings({ portal, s }: { portal: 'fleet' | 'station'; s?: StationData }) {
  const { T } = useI18n();
  const org = useOrg(portal === 'fleet').data;
  const st = s?.station;
  return (
    <>
      <Ph h={T('الإعدادات والضريبة', 'Settings & VAT')} p={T('تُحفظ التغييرات فوراً وتنعكس على التطبيقات والإيصالات.', 'Changes are saved instantly and apply to the apps and receipts.')} />
      <div className="grid2">
        {portal === 'fleet' ? (
          <div className="card panel">
            <h2>{T('بيانات الشركة', 'Company details')}</h2>
            {org && (
              <div className="mgrid" style={{ margin: 0 }}>
                <SettingField id="set-ar" label={T('الاسم بالعربية', 'Name (Arabic)')} value={org.name.ar} save={(v) => api.fleet.updateOrg({ name: { ar: v, en: org.name.en } })} />
                <SettingField id="set-en" label={T('الاسم بالإنجليزية', 'Name (English)')} value={org.name.en} ltr save={(v) => api.fleet.updateOrg({ name: { ar: org.name.ar, en: v } })} />
                <div className="full"><SettingField id="set-ph" label={T('هاتف العمليات', 'Operations phone')} value={org.phone} ltr mono save={(v) => api.fleet.updateOrg({ phone: v })} /></div>
              </div>
            )}
          </div>
        ) : (
          <div className="card panel">
            <h2>{T('بيانات المحطة', 'Station details')}</h2>
            {st && (
              <div className="mgrid" style={{ margin: 0 }}>
                <SettingField id="set-sar" label={T('الاسم بالعربية', 'Name (Arabic)')} value={st.name.ar} save={(v) => api.stations.update(st.id, { name: { ar: v } })} />
                <SettingField id="set-sen" label={T('الاسم بالإنجليزية', 'Name (English)')} value={st.name.en} ltr save={(v) => api.stations.update(st.id, { name: { en: v } })} />
                <SettingField id="set-aar" label={T('العنوان بالعربية', 'Address (Arabic)')} value={st.area.ar} save={(v) => api.stations.update(st.id, { area: { ar: v } })} />
                <SettingField id="set-aen" label={T('العنوان بالإنجليزية', 'Address (English)')} value={st.area.en} ltr save={(v) => api.stations.update(st.id, { area: { en: v } })} />
                <div className="full"><SettingField id="set-sph" label={T('هاتف المحطة', 'Station phone')} value={st.phone} ltr mono save={(v) => api.stations.update(st.id, { phone: v })} /></div>
              </div>
            )}
          </div>
        )}
        <VatCard />
      </div>
    </>
  );
}
