/**
 * Data hooks (TanStack Query) over the API client.
 *
 * Screens re-poll every few seconds, so a change made in one app (say, the
 * fleet manager approving a request on the web) shows up in the others (the
 * driver's phone) without a reload — the prototype's "shared state" demo,
 * now backed by the real services. Every mutation goes through useAction,
 * which shows a toast and refreshes all data.
 */
import { QueryClient, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { api, ApiError } from './api/client';
import type { NFn } from './i18n';
import { useI18n } from './i18n';
import { useToast } from './toast';

/** Poll interval for live data. */
export const LIVE_MS = 4000;

/** Creates the app's QueryClient with sensible defaults. */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        refetchInterval: LIVE_MS,
        refetchOnWindowFocus: true,
        staleTime: 1000,
        retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
      },
    },
  });
}

/** Text of an error in the current language. */
export function errorText(e: unknown, N: NFn): string {
  if (e instanceof ApiError) return N(e.text);
  return N({ ar: 'حدث خطأ غير متوقع.', en: 'Something went wrong.' });
}

/**
 * Returns run(fn, ok): performs a mutation, refreshes every query, and shows
 * either the success message or the server's error message. Resolves to the
 * result, or undefined on failure (errors never escape to the caller).
 */
export function useAction() {
  const qc = useQueryClient();
  const toast = useToast();
  const { N } = useI18n();
  return useCallback(
    async <T,>(fn: () => Promise<T>, ok?: string | ((r: T) => string)): Promise<T | undefined> => {
      try {
        const r = await fn();
        await qc.invalidateQueries();
        if (ok) toast(typeof ok === 'function' ? ok(r) : ok);
        return r;
      } catch (e) {
        toast(errorText(e, N));
        return undefined;
      }
    },
    [qc, toast, N],
  );
}

const q = <T,>(key: unknown[], fn: () => Promise<T>, enabled = true) => ({ queryKey: key, queryFn: fn, enabled });

// ---------- fleet ----------
export const useOrg = (on = true) => useQuery(q(['fleet', 'org'], api.fleet.org, on));
export const useTiers = (on = true) => useQuery(q(['fleet', 'tiers'], api.fleet.tiers, on));
export const useDrivers = (on = true) => useQuery(q(['fleet', 'drivers'], api.fleet.drivers, on));
export const useVehicles = (on = true) => useQuery(q(['fleet', 'vehicles'], api.fleet.vehicles, on));
export const useRules = (on = true) => useQuery(q(['fleet', 'rules'], api.fleet.rules, on));
export const useRequests = (on = true) => useQuery(q(['fleet', 'requests'], () => api.fleet.requests(), on));
export const useDriverMe = (on = true) => useQuery(q(['fleet', 'me'], api.fleet.me, on));

// ---------- stations ----------
export const useStations = (on = true) => useQuery(q(['stations'], api.stations.list, on));
export const usePrices = (on = true) => useQuery({ ...q(['stations', 'prices'], api.stations.prices, on), refetchInterval: 60_000 });
export const usePumps = (stationId: string | undefined) =>
  useQuery(q(['stations', stationId, 'pumps'], () => api.stations.pumps(stationId!), !!stationId));
export const useStaff = (stationId: string | undefined) =>
  useQuery(q(['stations', stationId, 'staff'], () => api.stations.staff(stationId!), !!stationId));
export const useStaffMe = (on = true) => useQuery(q(['stations', 'staff', 'me'], api.stations.me, on));

// ---------- payments ----------
export const useWallet = (on = true) => useQuery(q(['payments', 'wallet'], api.payments.wallet, on));
export const useTax = (on = true) => useQuery(q(['payments', 'tax'], api.payments.tax, on));
export const useSettlements = (on = true) => useQuery(q(['payments', 'settlements'], api.payments.settlements, on));

// ---------- fueling ----------
export const usePass = (on = true) => useQuery(q(['fueling', 'pass'], api.fueling.pass, on));
export const useTransactions = (on = true) => useQuery(q(['fueling', 'tx'], api.fueling.transactions, on));
export const useSecurityEvents = (on = true) => useQuery(q(['fueling', 'security'], api.fueling.securityEvents, on));
