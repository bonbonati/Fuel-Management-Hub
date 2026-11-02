/**
 * Declarative edit dialogs (port of the prototype's MODALS). Each spec lists
 * its fields and a save() that calls the API; EditModal renders the form,
 * applies the same HTML validation as the prototype, and shows server errors
 * (bilingual) inline.
 */
import { useQueryClient } from '@tanstack/react-query';
import {
  api, ApiError, FUELFULL, FUELS, roleName, shiftName, short, useI18n, useToast,
  type Fuel, type FleetData, type Lang, type NFn, type Pump, type Staff, type StationData, type TFn,
} from '@petro/shared';
import { createContext, useContext, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Icon } from '@petro/shared';

type Val = string | boolean | string[];
export interface FieldSpec {
  k: string;
  l: string;
  v: string | number | boolean | string[];
  t?: 'text' | 'number' | 'select' | 'multi' | 'check';
  req?: boolean;
  pattern?: string;
  min?: number;
  max?: number;
  step?: number;
  opts?: [string, string][];
  hint?: string;
  ltr?: boolean;
  mono?: boolean;
  full?: boolean;
  col?: boolean;
}
export interface ModalSpec {
  title: string;
  fields: FieldSpec[];
  /** Saves the values and returns the success toast. Throws ApiError on failure. */
  save: (v: Record<string, Val>) => Promise<string>;
}

/** Thrown by save() for client-side validation failures. */
export class FormError extends Error {}

export function EditModal({ spec, onClose }: { spec: ModalSpec; onClose: () => void }) {
  const { T, N } = useI18n();
  const toast = useToast();
  const qc = useQueryClient();
  const form = useRef<HTMLFormElement>(null);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const f = form.current!;
    const vals: Record<string, Val> = {};
    for (const x of spec.fields) {
      const id = 'mf-' + x.k;
      if (x.t === 'check') vals[x.k] = (f.querySelector('#' + id) as HTMLInputElement).checked;
      else if (x.t === 'multi') vals[x.k] = [...f.querySelectorAll<HTMLInputElement>(`input[name="${id}"]:checked`)].map((i) => i.value);
      else {
        const el = f.querySelector('#' + id) as HTMLInputElement | HTMLSelectElement;
        vals[x.k] = el.value.trim();
        if (!el.checkValidity() || (x.req && !vals[x.k])) {
          setErr(T(`يرجى مراجعة حقل «${x.l}»`, `Please check the "${x.l}" field`) + (x.hint ? ` · ${x.hint}` : ''));
          el.focus();
          return;
        }
      }
    }
    setBusy(true);
    try {
      const msg = await spec.save(vals);
      await qc.invalidateQueries();
      toast(msg);
      onClose();
    } catch (e) {
      setErr(e instanceof ApiError ? N(e.text) : e instanceof FormError ? e.message : T('حدث خطأ غير متوقع.', 'Something went wrong.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-bg" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <form className="modal" ref={form} role="dialog" aria-modal="true" aria-label={spec.title} noValidate onSubmit={submit}>
        <h3>{spec.title}</h3>
        <p className="sub" style={{ margin: 0 }}>{T('تنعكس التغييرات فوراً على تطبيقات الجوال.', 'Changes apply to the mobile apps immediately.')}</p>
        <div className="mgrid">{spec.fields.map((x) => <Field key={x.k} x={x} />)}</div>
        {err && <p className="rd" role="alert" style={{ fontWeight: 600, margin: '0 0 10px' }}>{err}</p>}
        <div className="row">
          <button type="button" className="btn-s" onClick={onClose}>{T('إلغاء', 'Cancel')}</button>
          <button type="submit" className="btn-p" style={{ background: 'var(--rc)' }} disabled={busy}><Icon name="check" />{T('حفظ', 'Save')}</button>
        </div>
      </form>
    </div>
  );
}

function Field({ x }: { x: FieldSpec }) {
  const id = 'mf-' + x.k;
  if (x.t === 'check')
    return (
      <label className={`checkrow ${x.full ? 'full' : ''}`} htmlFor={id}>
        <span>{x.l}</span>
        <input type="checkbox" className="toggle" id={id} defaultChecked={!!x.v} />
      </label>
    );
  if (x.t === 'multi')
    return (
      <div className="fieldw full">
        <span style={{ fontWeight: 600, fontSize: 13 }}>{x.l}</span>
        <div className={`multi${x.col ? ' col' : ''}`}>
          {x.opts!.map(([v, l]) => (
            <label key={v}><span>{l}</span><input type="checkbox" name={id} value={v} defaultChecked={(x.v as string[]).includes(v)} /></label>
          ))}
        </div>
      </div>
    );
  if (x.t === 'select')
    return (
      <div className={`fieldw ${x.full ? 'full' : ''}`}>
        <label htmlFor={id}>{x.l}</label>
        <select className="field" id={id} defaultValue={String(x.v)}>
          {x.opts!.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </div>
    );
  return (
    <div className={`fieldw ${x.full ? 'full' : ''}`}>
      <label htmlFor={id}>{x.l}</label>
      <input className={`field ${x.mono ? 'num' : ''}`} id={id} type={x.t === 'number' ? 'number' : 'text'} defaultValue={String(x.v)}
        required={x.req} min={x.min} max={x.max} step={x.step} pattern={x.pattern} dir={x.ltr ? 'ltr' : undefined} />
      {x.hint && <small className="hint">{x.hint}</small>}
    </div>
  );
}

// ---------- specs ----------

interface Ctx { T: TFn; N: NFn; lang: Lang }
const optsFuel = (N: NFn): [string, string][] => FUELS.map((k) => [k, N(FUELFULL[k])]);
const s = (v: Val) => v as string;
const b = (v: Val) => v as boolean;

export function vehicleSpec(id: string, d: FleetData, { T, N, lang }: Ctx): ModalSpec {
  const o = id === 'new' ? null : d.vehicles.find((v) => v.id === id) ?? null;
  return {
    title: o ? T('تعديل المركبة', 'Edit vehicle') : T('إضافة مركبة', 'Add vehicle'),
    fields: [
      { k: 'num', l: T('أرقام اللوحة', 'Plate number'), v: o?.plate.digits ?? '', req: true, mono: true, pattern: '[0-9]{1,5}', hint: T('أرقام فقط', 'Digits only') },
      { k: 'lat', l: T('رمز الولاية (لاتيني)', 'State code (Latin)'), v: o?.plate.latin ?? 'KRT', req: true, ltr: true },
      { k: 'ara', l: T('رمز الولاية (عربي)', 'State code (Arabic)'), v: o?.plate.arabic ?? 'خ ر ط', req: true },
      { k: 'fuel', l: T('نوع الوقود', 'Fuel grade'), t: 'select', v: o?.fuel ?? 'Diesel', opts: optsFuel(N) },
      { k: 'nar', l: T('الطراز بالعربية', 'Model (Arabic)'), v: o?.name.ar ?? '', req: true },
      { k: 'nen', l: T('الطراز بالإنجليزية', 'Model (English)'), v: o?.name.en ?? '', req: true, ltr: true },
      { k: 'tier', l: T('الفئة', 'Tier'), t: 'select', v: o ? o.tierId ?? '' : d.tiers[1]?.id ?? '',
        opts: [['', T('بدون فئة', 'No tier')], ...d.tiers.map((t): [string, string] => [t.id, `${N(t.name)} · ${short(t.budget, lang)}`])] },
      { k: 'custom', l: T('حد مخصص (ج.س)', 'Custom limit (SDG)'), t: 'number', v: o?.customLimit ?? '', min: 0, step: 10000,
        hint: T('اتركه فارغاً لاستخدام ميزانية الفئة', 'Leave empty to use the tier budget') },
      { k: 'driver', l: T('السائق', 'Driver'), t: 'select', v: o?.driverId ?? '',
        opts: [['', T('غير مخصص', 'Unassigned')], ...d.drivers.map((x): [string, string] => [x.id, N(x.name)])] },
      { k: 'tag', l: T('بطارية الشريحة (%)', 'Tag battery (%)'), t: 'number', v: o?.tagBattery ?? 100, min: 0, max: 100, req: true },
      { k: 'active', l: T('المركبة نشطة', 'Vehicle active'), t: 'check', v: o?.active ?? true, full: true },
    ],
    save: async (v) => {
      await api.fleet.saveVehicle(o?.id ?? null, {
        plate: { digits: s(v.num), latin: s(v.lat).toUpperCase(), arabic: s(v.ara) },
        name: { ar: s(v.nar), en: s(v.nen) }, fuel: s(v.fuel) as Fuel, tierId: s(v.tier) || null,
        customLimit: s(v.custom) === '' ? null : Number(v.custom), driverId: s(v.driver) || null,
        tagBattery: Math.max(0, Math.min(100, Number(v.tag))), active: b(v.active),
      });
      return o ? T('تم حفظ بيانات المركبة', 'Vehicle saved') : T('تمت إضافة المركبة وطلب شريحة الوقود', 'Vehicle added and fuel tag ordered');
    },
  };
}

export function driverSpec(id: string, d: FleetData, { T, N }: Ctx): ModalSpec {
  const o = id === 'new' ? null : d.drivers.find((x) => x.id === id) ?? null;
  const veh = o ? d.vehicles.find((v) => v.driverId === o.id) : undefined;
  return {
    title: o ? T('تعديل بيانات السائق', 'Edit driver') : T('إضافة سائق', 'Add driver'),
    fields: [
      { k: 'nar', l: T('الاسم بالعربية', 'Name (Arabic)'), v: o?.name.ar ?? '', req: true },
      { k: 'nen', l: T('الاسم بالإنجليزية', 'Name (English)'), v: o?.name.en ?? '', req: true, ltr: true },
      { k: 'code', l: T('رقم السائق', 'Driver ID'), v: o?.code ?? 'D-' + (3120 + d.drivers.length), req: true, mono: true },
      { k: 'phone', l: T('رقم الجوال', 'Mobile number'), v: o?.phone ?? '+249 ', req: true, mono: true, pattern: '\\+249[0-9 ]{9,13}',
        hint: T('يبدأ بـ +249 ويليه 9 أرقام', '+249 followed by 9 digits') },
      { k: 'veh', l: T('المركبة المخصصة', 'Assigned vehicle'), t: 'select', v: veh?.id ?? '', full: true,
        opts: [['', T('بدون مركبة', 'No vehicle')], ...d.vehicles.map((x): [string, string] => {
          const t = d.tiers.find((tt) => tt.id === x.tierId);
          return [x.id, `${N(x.name)} · ${x.plate.digits} ${x.plate.latin}${t ? ' · ' + N(t.name) : ''}`];
        })] },
      { k: 'app', l: T('تم تفعيل التطبيق', 'App activated'), t: 'check', v: o?.appActivated ?? false },
      { k: 'active', l: T('الحساب مفعّل', 'Account active'), t: 'check', v: o?.active ?? true },
    ],
    save: async (v) => {
      await api.fleet.saveDriver(o?.id ?? null, {
        name: { ar: s(v.nar), en: s(v.nen) }, code: s(v.code), phone: s(v.phone), appActivated: b(v.app), active: b(v.active), vehicleId: s(v.veh) || null,
      });
      return o ? T('تم حفظ بيانات السائق', 'Driver saved') : T('تمت إضافة السائق وإرسال دعوة التطبيق', 'Driver added and app invite sent');
    },
  };
}

export function tierSpec(id: string, d: FleetData, { T, N }: Ctx): ModalSpec {
  const o = id === 'new' ? null : d.tiers.find((t) => t.id === id) ?? null;
  const driverName = (driverId: string | null) => N(d.drivers.find((x) => x.id === driverId)?.name) || T('غير مخصص', 'Unassigned');
  return {
    title: o ? T('تعديل الفئة', 'Edit tier') : T('فئة جديدة', 'New tier'),
    fields: [
      { k: 'nar', l: T('اسم الفئة بالعربية', 'Tier name (Arabic)'), v: o?.name.ar ?? '', req: true },
      { k: 'nen', l: T('اسم الفئة بالإنجليزية', 'Tier name (English)'), v: o?.name.en ?? '', req: true, ltr: true },
      { k: 'budget', l: T('الميزانية الشهرية لكل مركبة (ج.س)', 'Monthly budget per vehicle (SDG)'), t: 'number', v: o?.budget ?? 1000000, min: 10000, step: 10000, req: true },
      { k: 'dailyL', l: T('الحد اليومي (لتر)', 'Daily cap (litres)'), t: 'number', v: o?.dailyLitres ?? 80, min: 1, req: true },
      { k: 'win', l: T('ساعات التعبئة', 'Fueling hours'), v: o?.window ?? '06:00–22:00', mono: true, req: true, full: true },
      { k: 'vehicles', l: T('المركبات والسائقون في هذه الفئة', 'Vehicles and drivers in this tier'), t: 'multi', col: true,
        v: o ? d.vehicles.filter((v) => v.tierId === o.id).map((v) => v.id) : [],
        opts: d.vehicles.map((v): [string, string] => {
          const cur = d.tiers.find((t) => t.id === v.tierId);
          const now = cur && (!o || cur.id !== o.id) ? ` (${T('حالياً في', 'now in')} ${N(cur.name)})` : '';
          return [v.id, `${N(v.name)} · ${v.plate.digits} ${v.plate.latin} — ${driverName(v.driverId)}${now}`];
        }) },
    ],
    save: async (v) => {
      const ids = v.vehicles as string[];
      await api.fleet.saveTier(o?.id ?? null, { name: { ar: s(v.nar), en: s(v.nen) }, budget: Number(v.budget), dailyLitres: Number(v.dailyL), window: s(v.win), vehicleIds: ids });
      return T(`تم حفظ الفئة · ${ids.length} مركبات مرتبطة`, `Tier saved · ${ids.length} vehicles assigned`);
    },
  };
}

export function staffSpec(id: string, sd: StationData, { T, N }: Ctx, all: Staff[]): ModalSpec {
  const o = id === 'new' ? null : all.find((x) => x.id === id) ?? null;
  return {
    title: o ? T('تعديل بيانات الموظف', 'Edit staff member') : T('إضافة موظف', 'Add staff member'),
    fields: [
      { k: 'nar', l: T('الاسم بالعربية', 'Name (Arabic)'), v: o?.name.ar ?? '', req: true },
      { k: 'nen', l: T('الاسم بالإنجليزية', 'Name (English)'), v: o?.name.en ?? '', req: true, ltr: true },
      { k: 'code', l: T('الرقم الوظيفي', 'Staff ID'), v: o?.code ?? 'ATT-' + (13 + all.length), req: true, mono: true },
      { k: 'role', l: T('الدور', 'Role'), t: 'select', v: o?.role ?? 'att', opts: [['sup', N(roleName('sup'))], ['att', N(roleName('att'))]] },
      { k: 'shift', l: T('الوردية', 'Shift'), t: 'select', v: o?.shift ?? 'A', opts: ['A', 'B', 'C'].map((x): [string, string] => [x, shiftName(x)]) },
      { k: 'dev', l: T('الجهاز', 'Device'), v: o?.device ?? '', ltr: true },
      { k: 'on', l: T('الحساب مفعّل', 'Account active'), t: 'check', v: o?.active ?? true, full: true },
    ],
    save: async (v) => {
      await api.stations.saveStaff(sd.stationId!, o?.id ?? null, {
        name: { ar: s(v.nar), en: s(v.nen) }, code: s(v.code), role: s(v.role) as Staff['role'], shift: s(v.shift) as Staff['shift'], device: s(v.dev), active: b(v.on),
      });
      return o ? T('تم حفظ بيانات الموظف', 'Staff member saved') : T('تمت إضافة الموظف', 'Staff member added');
    },
  };
}

export function pumpSpec(p: Pump, { T, N }: Ctx): ModalSpec {
  return {
    title: T(`تعديل المضخة ${p.number}`, `Edit pump ${p.number}`),
    fields: [
      { k: 'grades', l: T('أنواع الوقود', 'Fuel grades'), t: 'multi', v: p.grades, opts: optsFuel(N) },
      { k: 'flow', l: T('معدل التدفق (لتر/دقيقة)', 'Flow rate (L/min)'), t: 'number', v: p.flowRate, min: 10, max: 200, req: true },
      { k: 'on', l: T('المضخة تعمل', 'Pump online'), t: 'check', v: p.online },
    ],
    save: async (v) => {
      const grades = v.grades as Fuel[];
      if (!grades.length) throw new FormError(T('اختر نوع وقود واحداً على الأقل.', 'Choose at least one fuel grade.'));
      await api.stations.updatePump(p.stationId, p.number, { grades, flowRate: Number(v.flow), online: b(v.on) });
      return T('تم حفظ إعدادات المضخة', 'Pump saved');
    },
  };
}

// ---------- host ----------

export type ModalRef = { type: 'vehicle' | 'driver' | 'tier' | 'staff' | 'pump'; id: string };
const ModalCtx = createContext<(m: ModalRef) => void>(() => {});

/** Opens an edit dialog: openModal({type: 'vehicle', id: 'f3' | 'new'}). */
export const useOpenModal = () => useContext(ModalCtx);

export function ModalProvider({ children, render }: { children: ReactNode; render: (m: ModalRef, close: () => void) => ReactNode }) {
  const [m, setM] = useState<ModalRef | null>(null);
  return (
    <ModalCtx.Provider value={setM}>
      {children}
      {m && render(m, () => setM(null))}
    </ModalCtx.Provider>
  );
}

/** Builds the translation context for spec builders. */
export function useSpecCtx(): Ctx {
  const { T, N, lang } = useI18n();
  return { T, N, lang };
}
