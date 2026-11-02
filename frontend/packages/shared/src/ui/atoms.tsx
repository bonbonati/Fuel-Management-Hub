/**
 * Small presentational components shared by both apps. Markup and class
 * names match the prototype so the shared CSS (styles/petro.css) applies.
 */
import type { ReactNode } from 'react';
import { renderSVG } from 'uqr';
import type { DriverRequest, Fuel, Plate as PlateT, Transaction } from '../api/types';
import { FUEL, fuelClass, quotaCls } from '../domain';
import { arDig, cur, sar } from '../format';
import { useI18n } from '../i18n';
import { Icon } from './icons';

/** Sudanese number plate (Latin and Arabic halves, always LTR). */
export function PlateTag({ p }: { p: PlateT }) {
  return (
    <span className="plate">
      <span>
        {p.digits} {p.latin}
      </span>
      <span>
        {arDig(p.digits)} {p.arabic}
      </span>
    </span>
  );
}

/** Colored fuel grade tag. */
export function FuelTag({ f }: { f: Fuel }) {
  const { N } = useI18n();
  return <span className={`fg ${fuelClass(f)}`}>{N(FUEL[f])}</span>;
}

/** Usage bar (amber ≥85%, red ≥98%). */
export function Quota({ used, lim }: { used: number; lim: number }) {
  const p = Math.min(100, (used / Math.max(1, lim)) * 100);
  return (
    <div className="quota" role="presentation">
      <i className={quotaCls(p)} style={{ width: `${p}%` }} />
    </div>
  );
}

/** Amount with the currency caption. */
export function Money({ n, cls = '' }: { n: number; cls?: string }) {
  const { lang } = useI18n();
  return (
    <>
      <span className={`num ${cls}`}>{sar(n)}</span>
      <span className="cap">{cur(lang)}</span>
    </>
  );
}

/** Transaction status pill. */
export function StatusPill({ t }: { t: Pick<Transaction, 'status'> }) {
  const { T } = useI18n();
  if (t.status === 'done')
    return (
      <span className="pill em">
        <Icon name="checkc" size={13} />
        {T('مكتملة', 'Completed')}
      </span>
    );
  if (t.status === 'fueling')
    return (
      <span className="pill cy">
        <span className="dot pulse" />
        {T('جارٍ الصرف', 'Dispensing')}
      </span>
    );
  return (
    <span className="pill am">
      <span className="dot" />
      {T('معتمدة', 'Authorized')}
    </span>
  );
}

/** Request status pill. */
export function ReqPill({ r }: { r: Pick<DriverRequest, 'status'> }) {
  const { T } = useI18n();
  if (r.status === 'pending')
    return (
      <span className="pill am">
        <Icon name="hour" size={13} />
        {T('قيد المراجعة', 'Under review')}
      </span>
    );
  if (r.status === 'approved')
    return (
      <span className="pill em">
        <Icon name="checkc" size={13} />
        {T('معتمد', 'Approved')}
      </span>
    );
  return (
    <span className="pill rd">
      <Icon name="xc" size={13} />
      {T('مرفوض', 'Declined')}
    </span>
  );
}

/** Switch styled as the prototype's .toggle checkbox. */
export function Toggle({ checked, onChange, label, id, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; id?: string; disabled?: boolean }) {
  return (
    <input
      type="checkbox"
      className="toggle"
      id={id}
      checked={checked}
      disabled={disabled}
      aria-label={label}
      onChange={(e) => onChange(e.target.checked)}
    />
  );
}

/** Real, scannable QR code (SVG) for the driver's pass token. */
export function QrCode({ value, size = 196 }: { value: string; size?: number }) {
  const svg = renderSVG(value, { border: 2, ecc: 'M' });
  return (
    <div
      className="qrcode"
      role="img"
      aria-label="QR"
      style={{ width: size, height: size }}
      // uqr produces a self-contained <svg> from the token string only.
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

/** Empty-state block. */
export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}
