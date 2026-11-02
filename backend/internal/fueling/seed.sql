-- Demo data (SEED_DEMO_DATA=true): the prototype's recent fill-ups and
-- security log, timestamped relative to the first start. Idempotent.

INSERT INTO transactions (id, org_id, vehicle_id, driver_id, plate, station_id, pump, fuel, amount, price_per_litre, status, code, source, created_at, started_at, completed_at) VALUES
 ('T-90412', 'o1', 'f3', 'd3', '{"digits":"7781","latin":"KRT","arabic":"خ ر ط"}', 's1', 3, '95',     180000, 3600, 'done', '', 'qr',   now() - interval '2 hours',            now() - interval '2 hours',            now() - interval '115 minutes'),
 ('T-90377', 'o1', 'f3', 'd3', '{"digits":"7781","latin":"KRT","arabic":"خ ر ط"}', 's2', 5, '95',     144000, 3600, 'done', '', 'app',  now() - interval '1 day 2 hours',      now() - interval '1 day 2 hours',      now() - interval '1 day 115 minutes'),
 ('T-90301', 'o1', 'f1', 'd1', '{"digits":"4410","latin":"KRT","arabic":"خ ر ط"}', 's1', 1, 'Diesel', 225000, 3250, 'done', '', 'rfid', now() - interval '3 hours',            now() - interval '3 hours',            now() - interval '175 minutes'),
 ('T-90288', '',   '',   '',   '{"digits":"5521","latin":"KH","arabic":"خ"}',      's1', 6, '91',     90000,  3420, 'done', '', 'rfid', now() - interval '3 hours 30 minutes', now() - interval '3 hours 30 minutes', now() - interval '205 minutes'),
 ('T-90271', '',   '',   '',   '{"digits":"8830","latin":"OM","arabic":"أ م"}',    's1', 2, '95',     165000, 3600, 'done', '', 'rfid', now() - interval '4 hours',            now() - interval '4 hours',            now() - interval '235 minutes'),
 ('T-90250', 'o1', 'f4', 'd4', '{"digits":"6012","latin":"KH","arabic":"خ"}',      's3', 2, '91',     142000, 3420, 'done', '', 'rfid', now() - interval '1 day 5 hours',      now() - interval '1 day 5 hours',      now() - interval '1 day 295 minutes'),
 ('T-90233', 'o1', 'f6', 'd6', '{"digits":"9087","latin":"BH","arabic":"ب ح"}',    's1', 4, 'Diesel', 300000, 3250, 'done', '', 'rfid', now() - interval '1 day 6 hours',      now() - interval '1 day 6 hours',      now() - interval '1 day 355 minutes'),
 ('T-90210', 'o1', 'f2', 'd2', '{"digits":"2290","latin":"KH","arabic":"خ"}',      's2', 1, 'Diesel', 195000, 3250, 'done', '', 'rfid', now() - interval '1 day 10 hours',     now() - interval '1 day 10 hours',     now() - interval '1 day 595 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO security_events (org_id, severity, title, detail, ref, created_at)
SELECT * FROM (VALUES
 ('o1', 'critical', '{"ar":"إيقاف المضخة: نوع وقود غير مسموح","en":"Pump stopped: grade not allowed"}'::jsonb,
  '{"ar":"طلبت المركبة 9087 بنزين سوبر بدلاً من الجازولين المسجّل لها في طاقة بحري، فأُوقف الصرف.","en":"Vehicle 9087 BH requested super benzene instead of its registered diesel at Bahri Energy. Dispensing was stopped."}'::jsonb,
  '9087-BH', now() - interval '14 minutes'),
 ('o1', 'warning', '{"ar":"طلب يتجاوز سعة الخزان","en":"Request exceeds tank capacity"}'::jsonb,
  '{"ar":"طلبت المركبة 4410 تعبئة 140 لتراً لخزان سعته 76 لتراً، فتوقف المسدس عند حد الأمان.","en":"Vehicle 4410 KRT requested 140 L for a 76 L tank. The nozzle stopped at the safety limit."}'::jsonb,
  'RFID-4410', now() - interval '1 hour')
) AS v(org_id, severity, title, detail, ref, created_at)
WHERE NOT EXISTS (SELECT 1 FROM security_events);
