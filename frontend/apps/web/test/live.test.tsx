/**
 * Opt-in smoke test against a running stack (docker compose / k8s):
 *   LIVE_GATEWAY=http://localhost:8080 pnpm --filter @petro/web test
 * Renders the real app with the real API client and signs in as the demo
 * personas. Skipped when LIVE_GATEWAY is not set (unit runs and CI).
 */
import { AppProviders } from '@petro/shared';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { App } from '../src/App';

const gateway = process.env.LIVE_GATEWAY;

describe.skipIf(!gateway)('live stack', () => {
  const renderLive = () => {
    localStorage.setItem('petro-lang', 'en');
    return render(<AppProviders storageKey="petro-web-session" baseUrl={gateway}><App /></AppProviders>);
  };

  it('fleet manager sees real fleet data', { timeout: 20_000 }, async () => {
    renderLive();
    await userEvent.click(await screen.findByRole('button', { name: /Fleet manager/ }, { timeout: 8000 }));
    expect(await screen.findByRole('heading', { name: 'Fleet overview' }, { timeout: 8000 })).toBeInTheDocument();
    expect(await screen.findByText('Freight trucks', {}, { timeout: 8000 })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /^Vehicles/ }));
    expect(await screen.findByText('Mitsubishi Canter')).toBeInTheDocument();
  });

  it('station supervisor sees pumps and settlements', { timeout: 20_000 }, async () => {
    renderLive();
    await userEvent.click(await screen.findByRole('button', { name: /Station supervisor/ }, { timeout: 8000 }));
    expect(await screen.findByRole('heading', { name: 'Blue Nile Station' }, { timeout: 8000 })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /^Pumps/ }));
    expect(await screen.findAllByRole('checkbox', { name: /Pump online/ })).toHaveLength(8);
  });
});
