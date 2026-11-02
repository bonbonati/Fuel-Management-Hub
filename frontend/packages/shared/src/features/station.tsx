/**
 * Station-side data shared by the web station console and the mobile
 * attendant app (prototype: stTx, shiftStats, pumpState).
 */
import { api } from '../api/client';
import type { Transaction } from '../api/types';
import { DEFAULT_PRICES } from '../domain';
import { useI18n } from '../i18n';
import { useAction, usePrices, usePumps, useStaffMe, useTax, useTransactions } from '../queries';

/** The signed-in staff member's station, its pumps and transactions. */
export function useStationData() {
  const me = useStaffMe();
  const stationId = me.data?.station.id;
  const pumps = usePumps(stationId);
  const tx = useTransactions();
  const tax = useTax();
  const prices = usePrices();
  return {
    me: me.data?.staff,
    station: me.data?.station,
    stationId,
    pumps: pumps.data ?? [],
    tx: tx.data ?? [],
    tax: tax.data,
    prices: prices.data ?? DEFAULT_PRICES,
    loading: !me.data || !pumps.data || !tx.data,
  };
}
export type StationData = ReturnType<typeof useStationData>;

/** Completed fill-ups of the shift: count, sales, litres. */
export function shiftStats(tx: Transaction[]) {
  const d = tx.filter((t) => t.status === 'done');
  const sales = d.reduce((a, t) => a + t.amount, 0);
  return { d, sales, lit: d.reduce((a, t) => a + t.litres, 0) };
}

/** Start/finish actions for open transactions, with the prototype's messages. */
export function useTxActions() {
  const { T } = useI18n();
  const run = useAction();
  return {
    start: (t: Transaction) => run(() => api.fueling.start(t.id), T(`تم تشغيل المضخة ${t.pump}`, `Pump ${t.pump} started`)),
    finish: (t: Transaction) => run(() => api.fueling.finish(t.id), T(`اكتملت العملية · إيصال ${t.id}`, `Completed · receipt ${t.id}`)),
  };
}
