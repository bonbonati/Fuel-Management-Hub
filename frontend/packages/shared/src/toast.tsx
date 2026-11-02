/**
 * One transient status message at a time (prototype toast()), announced to
 * screen readers through role="status".
 */
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';

const Ctx = createContext<((msg: string) => void) | null>(null);

/** How long a toast stays visible. */
export const TOAST_MS = 2800;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [msg, setMsg] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const show = useCallback((m: string) => {
    setMsg(m);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setMsg(null), TOAST_MS);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  return (
    <Ctx.Provider value={show}>
      {children}
      {msg && (
        <div className="toast" role="status">
          {msg}
        </div>
      )}
    </Ctx.Provider>
  );
}

/** Returns the function that shows a toast. */
export function useToast(): (msg: string) => void {
  const v = useContext(Ctx);
  if (!v) throw new Error('useToast must be used inside <ToastProvider>');
  return v;
}
