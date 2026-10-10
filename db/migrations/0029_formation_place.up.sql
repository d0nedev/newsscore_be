-- A starter's slot in the formation, 1 (goalkeeper) to 11, in the order the
-- source draws the pitch: Flashscore's LL, SofaScore's order of starters.
-- NULL on a starter: not read yet (ingestor -backfill-lineups); 0: the source gives none.
-- Substitutes stay NULL.
ALTER TABLE match_lineups ADD COLUMN formation_place SMALLINT;
