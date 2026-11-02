import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, configureApi } from '../src/api/client';

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn(async () => new Response(body === undefined ? '' : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }));
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
  configureApi({ baseUrl: '', getToken: () => null });
});

describe('api client', () => {
  it('sends the bearer token and JSON body to the gateway path', async () => {
    const fetch = mockFetch(201, { id: 'R-1044' });
    configureApi({ baseUrl: 'https://gw.example', getToken: () => 'tok' });

    const r = await api.fleet.createRequest({ type: 'limit', amount: 200000, reason: 'trip', note: '' });

    expect(r).toEqual({ id: 'R-1044' });
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('https://gw.example/api/v1/fleet/requests');
    expect(init.method).toBe('POST');
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok');
    expect(JSON.parse(init.body as string)).toEqual({ type: 'limit', amount: 200000, reason: 'trip', note: '' });
  });

  it('omits Authorization when signed out and encodes path parameters', async () => {
    const fetch = mockFetch(200, {});
    await api.fleet.setVehicleActive('f/1', false);
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/v1/fleet/vehicles/f%2F1/status');
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined();
  });

  it('turns problem documents into ApiError with the bilingual message', async () => {
    mockFetch(409, { code: 'limit_exceeded', message: { ar: 'تجاوز', en: 'Exceeded' } });
    const err = await api.fueling.authorize({ stationId: 's1', pump: 3, fuel: '95', amount: 1 }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: 'limit_exceeded', text: { ar: 'تجاوز', en: 'Exceeded' } });
  });

  it('calls onUnauthorized on 401 when a token was sent', async () => {
    mockFetch(401, { code: 'unauthorized', message: { ar: 'x', en: 'x' } });
    const onUnauthorized = vi.fn();
    configureApi({ baseUrl: '', getToken: () => 'expired', onUnauthorized });
    await expect(api.fleet.org()).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it('reports network failures as status 0 with a bilingual message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('offline'); }));
    const err = (await api.stations.list().catch((e) => e)) as ApiError;
    expect(err.status).toBe(0);
    expect(err.code).toBe('network');
    expect(err.text.ar).not.toBe('');
  });

  it('builds query strings for filtered lists', async () => {
    const fetch = mockFetch(200, []);
    await api.fleet.requests('pending');
    expect((fetch.mock.calls[0] as unknown as [string])[0]).toBe('/api/v1/fleet/requests?status=pending');
  });
});
