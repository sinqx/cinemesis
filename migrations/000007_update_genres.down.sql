
DROP TABLE IF EXISTS movies_genres;

DROP TABLE IF EXISTS genres;

ALTER TABLE movies ADD COLUMN genres text[] NOT NULL,;
