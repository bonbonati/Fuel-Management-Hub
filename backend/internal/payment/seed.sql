-- Demo data (SEED_DEMO_DATA=true). Idempotent.

-- Al Namaa's prepaid balance.
INSERT INTO wallets (org_id, balance) VALUES ('o1', 18450000) ON CONFLICT DO NOTHING;

-- Usage already spent this month by each demo vehicle (the prototype's "used").
INSERT INTO ledger (org_id, vehicle_id, tx_id, kind, amount) VALUES
 ('o1', 'f1', 'opening-f1', 'opening_usage', 1236000),
 ('o1', 'f2', 'opening-f2', 'opening_usage', 646000),
 ('o1', 'f3', 'opening-f3', 'opening_usage', 768000),
 ('o1', 'f4', 'opening-f4', 'opening_usage', 742000),
 ('o1', 'f5', 'opening-f5', 'opening_usage', 307000),
 ('o1', 'f6', 'opening-f6', 'opening_usage', 1440000)
ON CONFLICT DO NOTHING;

-- Blue Nile Station payouts: tomorrow's scheduled one and three paid ones.
-- Only seeded once (relative dates would otherwise add rows every day).
INSERT INTO settlements (station_id, payout_date, amount, fee, status)
SELECT 's1', current_date + d, a, f, s
FROM (VALUES (1, 27630000, 119500, 'scheduled'), (0, 26978000, 116600, 'paid'),
             (-1, 28815000, 124500, 'paid'), (-2, 24960000, 108000, 'paid')) AS v(d, a, f, s)
WHERE NOT EXISTS (SELECT 1 FROM settlements WHERE station_id = 's1')
ON CONFLICT DO NOTHING;
