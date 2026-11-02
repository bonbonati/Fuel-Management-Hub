/**
 * Sign-in screen shared by both apps: username/password form plus, in demo
 * mode, one-tap "sign in as" buttons for the prototype personas.
 */
import { useQuery } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Role, User } from '../api/types';
import { useI18n } from '../i18n';
import { errorText } from '../queries';
import { useSession } from '../session';
import { Icon } from '../ui/icons';

export const ROLE_LABEL: Record<Role, [string, string]> = {
  driver: ['سائق', 'Driver'],
  fleet_manager: ['مدير الأسطول', 'Fleet manager'],
  attendant: ['عامل المحطة', 'Station attendant'],
  station_admin: ['مشرف المحطة', 'Station supervisor'],
};
const ROLE_ICON: Record<Role, string> = { driver: 'wallet', fleet_manager: 'truck', attendant: 'scan', station_admin: 'pump' };
const ROLE_COLOR: Record<Role, string> = { driver: 'em', fleet_manager: 'bl', attendant: 'or', station_admin: 'or' };

export function Login({ roles, title, subtitle }: { roles: Role[]; title: string; subtitle: string }) {
  const { T, N } = useI18n();
  const { login, demoLogin } = useSession();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const demo = useQuery({ queryKey: ['demo-accounts'], queryFn: api.auth.demoAccounts, refetchInterval: false, retry: false });
  // One persona per role is enough; drivers other than the main demo driver stay hidden.
  const personas = (demo.data ?? []).filter((u) => roles.includes(u.role)).filter((u, i, all) => all.findIndex((x) => x.role === u.role) === i);

  const run = async (fn: () => Promise<User>) => {
    setBusy(true);
    setError('');
    try {
      await fn(); // the app re-renders for the signed-in user (and explains if it is the wrong app)
    } catch (e) {
      setError(errorText(e, N));
    } finally {
      setBusy(false);
    }
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    void run(() => login(username, password));
  };

  return (
    <div className="login">
      <div className="card">
        <h1>{title}</h1>
        <p className="sub" style={{ margin: 0 }}>{subtitle}</p>
        <form onSubmit={submit}>
          <div className="fieldw">
            <label htmlFor="lg-user">{T('اسم المستخدم', 'Username')}</label>
            <input className="field num" id="lg-user" autoComplete="username" dir="ltr" value={username} onChange={(e) => setUsername(e.target.value)} required />
          </div>
          <div className="fieldw">
            <label htmlFor="lg-pass">{T('كلمة المرور', 'Password')}</label>
            <input className="field" id="lg-pass" type="password" autoComplete="current-password" dir="ltr" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </div>
          {error && <p className="rd" role="alert" style={{ margin: 0, fontWeight: 600 }}>{error}</p>}
          <button className="btn-p" type="submit" disabled={busy}>
            <Icon name="key" />
            {T('تسجيل الدخول', 'Sign in')}
          </button>
        </form>
      </div>
      {personas.length > 0 && (
        <div className="card">
          <span className="lbl">{T('وضع العرض · الدخول بنقرة واحدة', 'Demo mode · one-tap sign-in')}</span>
          <div className="personas" style={{ marginTop: 10 }}>
            {personas.map((u) => (
              <button key={u.id} className="persona" disabled={busy} onClick={() => void run(() => demoLogin(u.username))}>
                <span className={`ico ${ROLE_COLOR[u.role]}`}><Icon name={ROLE_ICON[u.role]} /></span>
                <span style={{ flex: 1 }}>
                  <b>{T(...ROLE_LABEL[u.role])}</b>
                  <small>{N(u.name)} · <span className="num">{u.username}</span></small>
                </span>
                <Icon name="chev" size={16} />
              </button>
            ))}
          </div>
          <p className="hint" style={{ margin: '10px 0 0' }}>
            {T('كلمة مرور الحسابات التجريبية: ', 'Demo account password: ')}<span className="num">demo1234</span>
          </p>
        </div>
      )}
    </div>
  );
}
