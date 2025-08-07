CREATE TABLE reviews (
    id          SERIAL PRIMARY KEY,
    user_name   TEXT NOT NULL,
    text        TEXT NOT NULL,
    rating      SMALLINT NOT NULL CHECK (rating >= 0 AND rating <= 10),
    upvotes     INTEGER NOT NULL DEFAULT 0,
    downvotes   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited      BOOLEAN NOT NULL DEFAULT FALSE,
    movie_id    BIGINT NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (user_id, movie_id) 
);


CREATE TYPE vote_type AS ENUM ('like', 'dislike');

CREATE TABLE review_votes (
    review_id  BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vote       vote_type NOT NULL,
    PRIMARY KEY (review_id, user_id)
);