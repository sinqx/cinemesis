package data

import (
	"cinemesis/internal/filters"
	"cinemesis/internal/validator"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Review struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	MovieID   int64     `json:"movie_id"`
	Text      string    `json:"text"`
	Rating    uint8     `json:"rating"`
	Upvotes   int32     `json:"upvotes"`
	Downvotes int32     `json:"downvotes"`
	Edited    bool      `json:"edited"`
	CreatedAt time.Time `json:"created_at"`
}

type ReviewWithUser struct {
	Review
	UserName        string `json:"user_name"`
	TotalVotes      int32  `json:"total_votes"`
	CurrentUserVote int    `json:"user_vote,omitempty"`
}

func ValidateReview(v *validator.Validator, review *Review) {
	v.Check(review.Text != "", "text", "must be provided")
	v.Check(len(review.Text) >= 10, "text", "must be at least 10 characters long")
	v.Check(len(review.Text) <= 500, "text", "must be less than 500 characters long")

	v.Check(review.Rating >= 1 && review.Rating <= 10, "rating", "must be between 1 and 10")

	v.Check(review.MovieID > 0, "movie_id", "must be a valid movie ID")
	v.Check(review.UserID > 0, "user_id", "must be a valid user ID")
}

type ReviewModel struct {
	DB *sql.DB
}

func (r ReviewModel) Insert(review *Review) error {
	query := `
		INSERT INTO reviews (user_id, movie_id, text, rating, edited)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return r.DB.QueryRowContext(ctx, query,
		review.UserID,
		review.MovieID,
		review.Text,
		review.Rating,
		review.Edited,
	).Scan(&review.ID, &review.CreatedAt)
}

func (r ReviewModel) Get(ctx context.Context, id int64, userID *int64) (*ReviewWithUser, error) {
	var userVoteJoin string
	var args []interface{}
	args = append(args, id)

	if userID != nil {
		userVoteJoin = `LEFT JOIN review_vote rv ON rv.review_id = r.id AND rv.user_id = $2`
		args = append(args, *userID)
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.user_id, r.movie_id, r.text, r.rating,
		       r.upvotes, r.downvotes, r.created_at, r.edited,
		       u.name AS user_name,
		       (r.upvotes - r.downvotes) AS total_votes,
		       COALESCE(rv.vote_type, 0) AS user_vote
		FROM reviews r
		JOIN users u ON r.user_id = u.id
		%s
		WHERE r.id = $1`, userVoteJoin)

	var review ReviewWithUser
	err := r.DB.QueryRowContext(ctx, query, args...).Scan(
		&review.ID,
		&review.UserID,
		&review.MovieID,
		&review.Text,
		&review.Rating,
		&review.Upvotes,
		&review.Downvotes,
		&review.CreatedAt,
		&review.Edited,
		&review.UserName,
		&review.TotalVotes,
		&review.CurrentUserVote,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &review, nil
}

func (r ReviewModel) GetFiltered(ctx context.Context, currentUserID int64, rf filters.ReviewFilters) ([]*ReviewWithUser, int, error) {
	query, args := filters.NewReviewQueryBuilder().Build(rf, currentUserID)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reviews []*ReviewWithUser
	var totalRecords int

	for rows.Next() {
		var review ReviewWithUser
		err := rows.Scan(
			&totalRecords,
			&review.ID,
			&review.UserID,
			&review.MovieID,
			&review.Text,
			&review.Rating,
			&review.CreatedAt,
			&review.Upvotes,
			&review.Downvotes,
			&review.Edited,
			&review.UserName,
			&review.TotalVotes,
			&review.CurrentUserVote,
		)
		if err != nil {
			return nil, 0, err
		}
		reviews = append(reviews, &review)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return reviews, totalRecords, nil
}

func (r ReviewModel) GetByUserID(ctx context.Context, userID int64) ([]Review, error) {
	if userID < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, user_id, movie_id, text, rating, upvotes, downvotes, edited, created_at
		FROM reviews
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.MovieID, &r.Text, &r.Rating,
			&r.Upvotes, &r.Downvotes, &r.Edited, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}

	if err = rows.Err(); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return reviews, nil
}

func (r ReviewModel) GetTopMovieReviews(ctx context.Context, movieID int64, limit int) ([]*ReviewWithUser, error) {
	query := `
		SELECT r.id, r.user_id, r.movie_id, LEFT(r.text, 300) AS text,
		       r.rating, r.upvotes, r.downvotes, r.created_at, r.edited,
		       u.name AS user_name,
		       (r.upvotes - r.downvotes) AS total_votes
		FROM reviews r
		JOIN users u ON r.user_id = u.id
		WHERE r.movie_id = $1 AND r.upvotes > 0
		ORDER BY r.upvotes DESC
		LIMIT $2
	`

	rows, err := r.DB.QueryContext(ctx, query, movieID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*ReviewWithUser
	for rows.Next() {
		var review ReviewWithUser
		err := rows.Scan(
			&review.ID, &review.UserID, &review.MovieID, &review.Text,
			&review.Rating, &review.Upvotes, &review.Downvotes,
			&review.CreatedAt, &review.Edited, &review.UserName, &review.TotalVotes,
		)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, &review)
	}

	return reviews, rows.Err()
}

func (r ReviewModel) Update(ctx context.Context, reviewID int64, review *Review) error {
	query := `
		UPDATE reviews
		SET text = $1, movie_id  = $2, user_id = $3, upvotes = $4, rating = $5, edited = true
		WHERE id = $6
		RETURNING id, created_at`

	args := []any{
		review.Text,
		review.MovieID,
		review.UserID,
		review.Upvotes,
		review.Rating,
		reviewID,
	}

	err := r.DB.QueryRowContext(ctx, query, args...).Scan(&review.ID, &review.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRecordNotFound
		}
		return err
	}

	return nil
}

func (r ReviewModel) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `DELETE FROM reviews WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}
