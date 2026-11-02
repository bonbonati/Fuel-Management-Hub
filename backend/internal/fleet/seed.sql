-- Demo data (SEED_DEMO_DATA=true): the prototype's Al Namaa Logistics fleet.
-- Idempotent: every insert is ON CONFLICT DO NOTHING, so restarts never
-- overwrite changes made through the apps.

INSERT INTO orgs (id, name, phone, city) VALUES
 ('o1', '{"ar":"شركة النماء للخدمات اللوجستية","en":"Al Namaa Logistics Co."}', '+249 91 230 4471', '{"ar":"الخرطوم","en":"Khartoum"}')
ON CONFLICT DO NOTHING;

INSERT INTO tiers (id, org_id, icon, color, name, budget, daily_litres, fuel_window) VALUES
 ('t1', 'o1', 'truck', 'or', '{"ar":"شاحنات النقل","en":"Freight trucks"}', 1500000, 200, '24/7'),
 ('t2', 'o1', 'bus',   'bl', '{"ar":"حافلات التوصيل","en":"Delivery vans"}', 1200000, 80, '06:00–21:00'),
 ('t3', 'o1', 'car',   'vi', '{"ar":"سيارات الإدارة والمبيعات","en":"Management & sales cars"}', 750000, 50, '07:00–22:00')
ON CONFLICT DO NOTHING;

INSERT INTO drivers (id, org_id, name, phone, code, app_activated, active) VALUES
 ('d1', 'o1', '{"ar":"فيصل عثمان","en":"Faisal Osman"}',        '+249 91 234 2114', 'D-3012', true,  true),
 ('d2', 'o1', '{"ar":"عمر الطيب","en":"Omar Altayeb"}',         '+249 92 118 8730', 'D-3025', true,  true),
 ('d3', 'o1', '{"ar":"عبدالرحمن حسن","en":"Abdurahman Hassan"}', '+249 90 552 4471', 'D-3071', true,  true),
 ('d4', 'o1', '{"ar":"سعد إبراهيم","en":"Saad Ibrahim"}',        '+249 99 407 0952', 'D-3088', true,  true),
 ('d5', 'o1', '{"ar":"يوسف بابكر","en":"Yousif Babiker"}',       '+249 12 960 3318', 'D-3102', false, true),
 ('d6', 'o1', '{"ar":"حمد الأمين","en":"Hamad Alamin"}',         '+249 11 773 7605', 'D-3117', true,  true)
ON CONFLICT DO NOTHING;

INSERT INTO vehicles (id, org_id, plate, name, driver_id, fuel, tier_id, custom_limit, extra, active, tag_battery) VALUES
 ('f1', 'o1', '{"digits":"4410","latin":"KRT","arabic":"خ ر ط"}', '{"ar":"إيسوزو دي ماكس","en":"Isuzu D-Max"}',      'd1', 'Diesel', 't1', NULL,    0,      true, 92),
 ('f2', 'o1', '{"digits":"2290","latin":"KH","arabic":"خ"}',      '{"ar":"تويوتا هايلكس","en":"Toyota Hilux"}',      'd2', 'Diesel', 't1', NULL,    0,      true, 11),
 ('f3', 'o1', '{"digits":"7781","latin":"KRT","arabic":"خ ر ط"}', '{"ar":"هيونداي H1","en":"Hyundai H1"}',          'd3', '95',     't2', NULL,    150000, true, 88),
 ('f4', 'o1', '{"digits":"6012","latin":"KH","arabic":"خ"}',      '{"ar":"تويوتا كورولا","en":"Toyota Corolla"}',    'd4', '91',     't3', NULL,    0,      true, 76),
 ('f5', 'o1', '{"digits":"3355","latin":"OM","arabic":"أ م"}',    '{"ar":"تويوتا هايس","en":"Toyota HiAce"}',        'd5', '95',     't2', NULL,    0,      true, 64),
 ('f6', 'o1', '{"digits":"9087","latin":"BH","arabic":"ب ح"}',    '{"ar":"ميتسوبيشي كانتر","en":"Mitsubishi Canter"}', 'd6', 'Diesel', 't1', 1800000, 0,      true, 95)
ON CONFLICT DO NOTHING;

INSERT INTO rules (org_id, network, anti_siphon, geofence, wrong_fuel, odometer, max_fill, max_fill_amount, hours, network_station_ids) VALUES
 ('o1', true, true, true, true, true, true, 400000, true, '{s1,s2}')
ON CONFLICT DO NOTHING;

INSERT INTO requests (id, org_id, driver_id, vehicle_id, type, amount, reason, note, status, approved, reply, created_at, decided_at) VALUES
 ('R-1043', 'o1', 'd4', 'f4', 'limit', 200000, 'trip',
  '{"ar":"رحلة توصيل إلى ود مدني يوم الثلاثاء.","en":"Delivery trip to Wad Madani on Tuesday."}',
  'pending', 0, '{"ar":"","en":""}', now() - interval '2 hours', NULL),
 ('R-1038', 'o1', 'd3', 'f3', 'limit', 150000, 'extra',
  '{"ar":"مهام توصيل إضافية خلال الأسبوع.","en":"Additional delivery assignments this week."}',
  'approved', 150000, '{"ar":"معتمد لهذا الشهر فقط.","en":"Approved for this month only."}', now() - interval '12 days', now() - interval '12 days')
ON CONFLICT DO NOTHING;
