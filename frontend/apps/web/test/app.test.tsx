import { AppProviders } from '@petro/shared';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { App } from '../src/App';
import { callsTo, fakeGateway, signIn, users } from './fixtures';

const renderApp = () => render(<AppProviders storageKey="petro-web-session"><App /></AppProviders>);

describe('sign-in', () => {
  it('offers only the web personas and opens the fleet console', async () => {
    const fetch = fakeGateway();
    localStorage.setItem('petro-lang', 'en');
    renderApp();

    const persona = await screen.findByRole('button', { name: /Fleet manager/ });
    expect(screen.getByRole('button', { name: /Station supervisor/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Driver/ })).not.toBeInTheDocument();

    await userEvent.click(persona);
    expect(await screen.findByRole('heading', { name: 'Fleet overview' })).toBeInTheDocument();
    expect(callsTo(fetch, 'POST', '/api/v1/auth/demo-login')).toEqual([{ username: 'fleet.manager' }]);
    // KPI from the wallet service.
    expect(await screen.findByText('18,450,000')).toBeInTheDocument();
  });

  it('explains that mobile-only roles belong to the (upcoming) mobile app', async () => {
    fakeGateway();
    signIn(users.drv);
    renderApp();
    expect(await screen.findByText('This account is for the mobile app, which is under development.')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Sign out' })).toHaveLength(2);
  });

  it('shows the server error for a bad password', async () => {
    fakeGateway({
      'POST /api/v1/auth/login': () => { throw { status: 401, code: 'invalid_credentials', message: { ar: 'خطأ', en: 'The username or password is incorrect.' } }; },
    });
    localStorage.setItem('petro-lang', 'en');
    renderApp();
    await userEvent.type(screen.getByLabelText('Username'), 'fleet.manager');
    await userEvent.type(screen.getByLabelText('Password'), 'wrong');
    await userEvent.click(screen.getByRole('button', { name: /Sign in/ }));
    expect(await screen.findByRole('alert')).toHaveTextContent('The username or password is incorrect.');
  });
});

describe('fleet console', () => {
  it('navigates with the sidebar and keeps the page in the URL hash', async () => {
    fakeGateway();
    signIn(users.fm);
    renderApp();
    await screen.findByRole('heading', { name: 'Fleet overview' });
    await userEvent.click(screen.getByRole('button', { name: /^Vehicles/ }));
    expect(await screen.findByRole('heading', { name: 'Vehicles' })).toBeInTheDocument();
    expect(window.location.hash).toBe('#vehicles');
    expect(screen.getByText('Isuzu D-Max')).toBeInTheDocument();
  });

  it('suspends a vehicle through the API', async () => {
    const fetch = fakeGateway({ 'PATCH /api/v1/fleet/vehicles/f3/status': (b) => ({ id: 'f3', ...(b as object) }) });
    signIn(users.fm);
    window.location.hash = 'vehicles';
    renderApp();
    const toggles = await screen.findAllByRole('checkbox', { name: 'Vehicle active' });
    await userEvent.click(toggles[1]);
    await waitFor(() => expect(callsTo(fetch, 'PATCH', '/api/v1/fleet/vehicles/f3/status')).toEqual([{ active: false }]));
    expect(await screen.findByRole('status')).toHaveTextContent('Vehicle suspended; its driver can no longer fuel');
  });

  it('approves a request with an adjusted amount', async () => {
    const fetch = fakeGateway({ 'POST /api/v1/fleet/requests/R-1043/decision': (b) => ({ ...(b as object), approved: 250000 }) });
    signIn(users.fm);
    window.location.hash = 'requests';
    renderApp();
    const amount = await screen.findByLabelText(/Approved amount/);
    await userEvent.clear(amount);
    await userEvent.type(amount, '250000');
    await userEvent.click(screen.getByRole('button', { name: 'Approve' }));
    await waitFor(() => expect(callsTo(fetch, 'POST', '/api/v1/fleet/requests/R-1043/decision')).toEqual([{ approve: true, amount: 250000, note: '' }]));
    expect(await screen.findByRole('status')).toHaveTextContent('250,000 SDG approved for Faisal Osman');
  });

  it('validates the vehicle form before calling the API', async () => {
    const fetch = fakeGateway({ 'POST /api/v1/fleet/vehicles': (b) => b });
    signIn(users.fm);
    window.location.hash = 'vehicles';
    renderApp();
    await userEvent.click(await screen.findByRole('button', { name: /Add vehicle/ }));
    const dialog = screen.getByRole('dialog', { name: 'Add vehicle' });
    await userEvent.type(within(dialog).getByLabelText('Plate number'), '12ab');
    await userEvent.click(within(dialog).getByRole('button', { name: /Save/ }));
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Please check the "Plate number" field');
    expect(callsTo(fetch, 'POST', '/api/v1/fleet/vehicles')).toHaveLength(0);

    const num = within(dialog).getByLabelText('Plate number');
    await userEvent.clear(num);
    await userEvent.type(num, '1234');
    await userEvent.type(within(dialog).getByLabelText('Model (Arabic)'), 'نيسان');
    await userEvent.type(within(dialog).getByLabelText('Model (English)'), 'Nissan');
    await userEvent.click(within(dialog).getByRole('button', { name: /Save/ }));
    await waitFor(() => expect(callsTo(fetch, 'POST', '/api/v1/fleet/vehicles')).toHaveLength(1));
    expect(callsTo(fetch, 'POST', '/api/v1/fleet/vehicles')[0]).toMatchObject({
      plate: { digits: '1234', latin: 'KRT', arabic: 'خ ر ط' }, name: { ar: 'نيسان', en: 'Nissan' }, tierId: 't2', customLimit: null, active: true,
    });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('lists alerts and runs their actions', async () => {
    fakeGateway();
    signIn(users.fm);
    window.location.hash = 'alerts';
    renderApp();
    // f3: low tag battery (11%); R-1043 pending.
    expect(await screen.findByText('Low tag battery')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Order a replacement tag' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Replacement tag ordered');
    await userEvent.click(screen.getByRole('button', { name: 'Review request' }));
    expect(await screen.findByRole('heading', { name: 'Driver requests' })).toBeInTheDocument();
  });
});

describe('station console', () => {
  it('shows pumps and takes one offline', async () => {
    const fetch = fakeGateway({ 'PATCH /api/v1/stations/s1/pumps/1/status': (b) => b });
    signIn(users.sa);
    window.location.hash = 'pumps';
    renderApp();
    const toggle = await screen.findByRole('checkbox', { name: 'Pump online 1' });
    await userEvent.click(toggle);
    await waitFor(() => expect(callsTo(fetch, 'PATCH', '/api/v1/stations/s1/pumps/1/status')).toEqual([{ online: false }]));
    expect(await screen.findByRole('status')).toHaveTextContent('Pump 1 taken offline');
  });

  it('starts an app authorization from the overview', async () => {
    const fetch = fakeGateway({ 'POST /api/v1/fueling/transactions/T-90413/start': () => ({}) });
    signIn(users.sa);
    renderApp();
    await userEvent.click(await screen.findByRole('button', { name: 'Start' }));
    await waitFor(() => expect(callsTo(fetch, 'POST', '/api/v1/fueling/transactions/T-90413/start')).toHaveLength(1));
    expect(await screen.findByRole('status')).toHaveTextContent('Pump 1 started');
  });

  it('shows the next settlement payout', async () => {
    fakeGateway();
    signIn(users.sa);
    window.location.hash = 'settle';
    renderApp();
    expect(await screen.findByText('27,630,000')).toBeInTheDocument();
    expect(screen.getByText(/•• 0917/)).toBeInTheDocument();
  });
});
