-- Demo data (SEED_DEMO_DATA=true): the prototype's three Khartoum stations.
-- s1 "Blue Nile Station" is the partner station whose staff use the apps.
-- Idempotent (ON CONFLICT DO NOTHING).

INSERT INTO stations (id, name, area, distance_km, wait, phone, bank_mask) VALUES
 ('s1', '{"ar":"محطة النيل الأزرق","en":"Blue Nile Station"}', '{"ar":"شارع أفريقيا","en":"Africa Rd"}', 1.2, '{"ar":"بدون انتظار","en":"No queue"}', '+249 18 377 0001', '0917'),
 ('s2', '{"ar":"وقود المقرن","en":"Al Mogran Fuel"}', '{"ar":"شارع النيل","en":"Nile St"}', 2.6, '{"ar":"انتظار 4 دقائق","en":"4 min wait"}', '+249 18 377 0002', ''),
 ('s3', '{"ar":"طاقة بحري","en":"Bahri Energy"}', '{"ar":"المنطقة الصناعية، بحري","en":"Industrial Area, Bahri"}', 4.8, '{"ar":"انتظار 7 دقائق","en":"7 min wait"}', '+249 18 377 0003', '')
ON CONFLICT DO NOTHING;

-- s1: pumps 1–4 benzene, 5–6 all grades, 7–8 high-flow diesel; pump 7 is offline.
INSERT INTO pumps (station_id, number, grades, flow_rate, online)
SELECT 's1', n,
       CASE WHEN n <= 4 THEN '{91,95}'::text[] WHEN n <= 6 THEN '{91,95,Diesel}'::text[] ELSE '{Diesel}'::text[] END,
       CASE WHEN n >= 7 THEN 120 ELSE 40 END,
       n <> 7
FROM generate_series(1, 8) AS n
ON CONFLICT DO NOTHING;
INSERT INTO pumps (station_id, number, grades, flow_rate, online)
SELECT 's2', n, '{91,95,Diesel}', 40, true FROM generate_series(1, 6) AS n
ON CONFLICT DO NOTHING;
INSERT INTO pumps (station_id, number, grades, flow_rate, online)
SELECT 's3', n, '{91,95,Diesel}', 40, true FROM generate_series(1, 12) AS n
ON CONFLICT DO NOTHING;

INSERT INTO staff (id, station_id, name, code, role, shift, device, active) VALUES
 ('a1', 's1', '{"ar":"مجدي الشيخ","en":"Magdi Alsheikh"}',   'ATT-01', 'sup', 'A', 'iPhone 13',  true),
 ('a2', 's1', '{"ar":"إبراهيم بابكر","en":"Ibrahim Babiker"}', 'ATT-09', 'att', 'A', 'Galaxy A54', true),
 ('a3', 's1', '{"ar":"أنور عبدالله","en":"Anwar Abdalla"}',   'ATT-12', 'att', 'B', 'Galaxy A34', false)
ON CONFLICT DO NOTHING;

INSERT INTO fuel_prices (grade, price) VALUES ('91', 3420), ('95', 3600), ('Diesel', 3250)
ON CONFLICT DO NOTHING;
