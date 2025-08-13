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
	UserName  string    `json:"user_name"`
	Text      string    `json:"text"`
	Rating    uint8     `json:"rating"`
	Upvotes   int32     `json:"upvotes,omitempty"`
	Downvotes int32     `json:"downvotes,omitempty"`
	CreatedAt time.Time `json:"-"`
	Edited    bool      `json:"edited,omitempty"`
	MovieID   int64     `json:"movie_id"`
	UserID    int64     `json:"user_id"`
}

type ReviewInput struct {
	ID       int64  `json:"id,omitempty"`
	Text     string `json:"text"`
	Rating   uint8  `json:"rating"`
	UserName string `json:"user_name"`
	Edited   bool   `json:"edited,omitempty"`
	MovieID  int64  `json:"movie_id"`
	UserID   int64  `json:"user_id"`
}

type ReviewResponse struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Text       string    `json:"text"`
	UserName   string    `json:"user_name"`
	Upvotes    int32     `json:"upvotes"`
	Downvotes  int32     `json:"downvotes"`
	TotalVotes int32     `json:"total_votes"`
	Rating     uint8     `json:"rating"`
	UserVote   VoteType  `json:"user_vote,omitempty"`
	Edited     bool      `json:"edited"`
	CreatedAt  time.Time `json:"created_at"`
}

func ValidateReview(v *validator.Validator, review *ReviewInput) {
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

func (r ReviewModel) UserAlreadyReviewed(ctx context.Context, userID, movieID int64) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM reviews WHERE user_id = $1 AND movie_id = $2)`
	err := r.DB.QueryRowContext(ctx, query, userID, movieID).Scan(&exists)
	return exists, err
}

func (r ReviewModel) Insert(review *ReviewInput) error {
	query := `
		INSERT INTO reviews (text, rating, user_name, edited, movie_id, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{review.Text, review.Rating, review.UserName, review.Edited, review.MovieID, review.UserID}

	return r.DB.QueryRowContext(ctx, query, args...).Scan(&review.ID)
}
func (r ReviewModel) Get(ctx context.Context, reviewID int64, userID *int64) (*ReviewResponse, error) {
	if reviewID < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
        SELECT r.id, r.user_id, r.user_name, r.text, r.edited,
		 r.rating, r.upvotes, r.downvotes, r.created_at,
         %s as user_vote
        FROM reviews r
        %s
        WHERE r.id = $1`

	args := []any{reviewID}
	var joinStr string
	var voteColumn string

	if userID != nil {
		joinStr = "LEFT JOIN review_votes rv ON r.id = rv.review_id AND rv.user_id = $2"
		voteColumn = "COALESCE(rv.vote_type, 0)"
		args = append(args, *userID)
	} else {
		voteColumn = "0"
		joinStr = ""
	}

	query = fmt.Sprintf(query, voteColumn, joinStr)

	var review ReviewResponse
	err := r.DB.QueryRowContext(ctx, query, args...).Scan(
		&review.ID,
		&review.UserID,
		&review.UserName,
		&review.Text,
		&review.Edited,
		&review.Rating,
		&review.Upvotes,
		&review.Downvotes,
		&review.CreatedAt,
		&review.UserVote,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &review, nil
}

func (r ReviewModel) GetByMovieIDFiltered(ctx context.Context, userID *int64, rf filters.ReviewFilters) ([]*ReviewResponse, int, error) {
	if userID != nil {
		rf.UserID = *userID
	}

	query, args := filters.NewReviewQueryBuilder().
		WithMovieID(rf.MovieID).
		WithRatingRange(rf.MinRating, rf.MaxRating).
		WithMinUpvotes(rf.MinUpvotes).
		WithDateRange(rf.DateFrom, rf.DateTo).
		Build(rf)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reviews []*ReviewResponse
	var totalRecords int

	for rows.Next() {
		var review ReviewResponse

		err := rows.Scan(
			&totalRecords,
			&review.ID,
			&review.UserName,
			&review.Text,
			&review.Rating,
			&review.CreatedAt,
			&review.Upvotes,
			&review.Downvotes,
			&review.Edited,
			&review.TotalVotes,
			&review.UserVote,
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

func (r ReviewModel) GetByUserID(ctx context.Context, userID int64) (*[]ReviewResponse, error) {
	if userID < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, user_name, text, rating, created_at, upvotes, edited
		FROM review
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []ReviewResponse
	for rows.Next() {
		var r ReviewResponse
		if err := rows.Scan(&r.ID, &r.Text, &r.UserName, &r.Rating, &r.CreatedAt, &r.Upvotes, &r.Edited); err != nil {
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

	return &reviews, nil
}

func (r ReviewModel) GetTopMovieReviews(ctx context.Context, movieID int64, limit int) ([]*ReviewResponse, error) {
	query := `
		SELECT id,
		       user_name,
		       LEFT(text, 300) AS text,
		       rating,
		       upvotes,
		       created_at
		FROM reviews 
		WHERE movie_id = $1 AND upvotes > 0
		ORDER BY upvotes DESC
		LIMIT $2
	`

	rows, err := r.DB.QueryContext(ctx, query, movieID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*ReviewResponse
	for rows.Next() {
		var review ReviewResponse
		err := rows.Scan(
			&review.ID,
			&review.UserName,
			&review.Text,
			&review.Rating,
			&review.Upvotes,
			&review.CreatedAt,
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
		SET text = $1, user_name = $2, rating = $3, upvotes = $4, edited = $5, movie_id = $6, user_id = $7
		WHERE id = $8
		RETURNING id, created_at`

	args := []any{
		review.Text,
		review.UserName,
		review.Rating,
		review.Upvotes,
		review.Edited,
		review.MovieID,
		review.UserID,
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
