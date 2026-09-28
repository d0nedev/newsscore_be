-- National team pages are scraped only for flagged countries, so adding a
-- country's league (J1, Malaysia) does not pull in its national team too.
ALTER TABLE countries ADD COLUMN scrape_national_team BOOLEAN NOT NULL DEFAULT false;
UPDATE countries SET scrape_national_team = true WHERE slug = 'indonesia';

-- Slugs follow Flashscore's national team slugs, so a national team seen later maps to the same row.
INSERT INTO countries (slug, name, sort_order) VALUES
    ('england', 'England', 10), ('spain', 'Spain', 11), ('italy', 'Italy', 12),
    ('germany', 'Germany', 13), ('france', 'France', 14), ('netherlands', 'Netherlands', 15),
    ('portugal', 'Portugal', 16), ('turkey', 'Turkey', 17), ('saudi-arabia', 'Saudi Arabia', 18),
    ('europe', 'Europe', 5), ('japan', 'Japan', 20), ('south-korea', 'South Korea', 21),
    ('thailand', 'Thailand', 22), ('malaysia', 'Malaysia', 23), ('usa', 'USA', 30), ('brazil', 'Brazil', 31)
ON CONFLICT (slug) DO UPDATE SET sort_order = EXCLUDED.sort_order;

-- UEFA's club competitions play a single league phase since 2024, so they are
-- 'league' tables; the knockout rounds land in matches.phase.
INSERT INTO competitions (slug, name, country_id, type, flashscore_path, sort_order)
SELECT v.slug, v.name, n.id, v.type, v.path, v.sort_order
FROM (VALUES
    ('champions-league',      'Champions League',          'europe',       'league', '/football/europe/champions-league/',                  10),
    ('europa-league',         'Europa League',             'europe',       'league', '/football/europe/europa-league/',                     11),
    ('conference-league',     'Conference League',         'europe',       'league', '/football/europe/conference-league/',                 12),
    ('premier-league',        'Premier League',            'england',      'league', '/football/england/premier-league/',                   20),
    ('laliga',                'LaLiga',                    'spain',        'league', '/football/spain/laliga/',                             21),
    ('serie-a',               'Serie A',                   'italy',        'league', '/football/italy/serie-a/',                            22),
    ('bundesliga',            'Bundesliga',                'germany',      'league', '/football/germany/bundesliga/',                       23),
    ('ligue-1',               'Ligue 1',                   'france',       'league', '/football/france/ligue-1/',                           24),
    ('eredivisie',            'Eredivisie',                'netherlands',  'league', '/football/netherlands/eredivisie/',                   25),
    ('liga-portugal',         'Liga Portugal',             'portugal',     'league', '/football/portugal/liga-portugal/',                   26),
    ('england-championship',  'Championship',              'england',      'league', '/football/england/championship/',                     27),
    ('super-lig',             'Süper Lig',                 'turkey',       'league', '/football/turkey/super-lig/',                         28),
    ('saudi-pro-league',      'Saudi Pro League',          'saudi-arabia', 'league', '/football/saudi-arabia/saudi-professional-league/',   29),
    ('j1-league',             'J1 League',                 'japan',        'league', '/football/japan/j1-league/',                          30),
    ('k-league-1',            'K League 1',                'south-korea',  'league', '/football/south-korea/k-league-1/',                   31),
    ('thai-league-1',         'Thai League 1',             'thailand',     'league', '/football/thailand/thai-league-1/',                   32),
    ('malaysia-super-league', 'Malaysia Super League',     'malaysia',     'league', '/football/malaysia/super-league/',                    33),
    ('mls',                   'MLS',                       'usa',          'league', '/football/usa/mls/',                                  40),
    ('brazil-serie-a',        'Brasileirão Série A',       'brazil',       'league', '/football/brazil/serie-a-betano/',                    41)
) AS v(slug, name, country, type, path, sort_order)
JOIN countries n ON n.slug = v.country;
