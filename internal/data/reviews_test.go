package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"cinemesis/internal/filters"
	"cinemesis/internal/validator"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReview(t *testing.T) {
	tests := []struct {
		name    string
		review  *Review
		wantErr bool
		errors  map[string]string
	}{
		{
			name: "Valid review",
			review: &Review{
				Text:    "This is a great movie!",
				Rating:  8,
				MovieID: 1,
				UserID:  1,
			},
			wantErr: false,
		},
		{
			name: "Empty text",
			review: &Review{
				Text:    "",
				Rating:  8,
				MovieID: 1,
				UserID:  1,
			},
			wantErr: true,
			errors: map[string]string{
				"text": "must be provided",
			},
		},
		{
			name: "Text too short",
			review: &Review{
				Text:    "Short",
				Rating:  8,
				MovieID: 1,
				UserID:  1,
			},
			wantErr: true,
			errors: map[string]string{
				"text": "must be at least 10 characters long",
			},
		},
		{
			name: "Text too long",
			review: &Review{
				Text:    strings.Repeat("a", 501),
				Rating:  8,
				MovieID: 1,
				UserID:  1,
			},
			wantErr: true,
			errors: map[string]string{
				"text": "must be less than 500 characters long",
			},
		},
		{
			name: "Invalid rating",
			review: &Review{
				Text:    "This is a great movie!",
				Rating:  11,
				MovieID: 1,
				UserID:  1,
			},
			wantErr: true,
			errors: map[string]string{
				"rating": "must be between 1 and 10",
			},
		},
		{
			name: "Invalid movie ID",
			review: &Review{
				Text:    "This is a great movie!",
				Rating:  8,
				MovieID: 0,
				UserID:  1,
			},
			wantErr: true,
			errors: map[string]string{
				"movie_id": "must be a valid movie ID",
			},
		},
		{
			name: "Invalid user ID",
			review: &Review{
				Text:    "This is a great movie!",
				Rating:  8,
				MovieID: 1,
				UserID:  0,
			},
			wantErr: true,
			errors: map[string]string{
				"user_id": "must be a valid user ID",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidateReview(v, tt.review)
			if tt.wantErr {
				assert.False(t, v.Valid())
				assert.Equal(t, tt.errors, v.Errors)
			} else {
				assert.True(t, v.Valid())
			}
		})
	}
}

func TestReviewModel_Insert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	review := &Review{
		UserID:  1,
		MovieID: 1,
		Text:    "Great movie!",
		Rating:  8,
		Edited:  false,
	}

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        INSERT INTO reviews (user_id, movie_id, text, rating, edited)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, created_at`)).
			WithArgs(review.UserID, review.MovieID, review.Text, review.Rating, review.Edited).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
				AddRow(1, fixedCreatedAt))

		err := m.Insert(review)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), review.ID)
		assert.Equal(t, fixedCreatedAt, review.CreatedAt)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Database error", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        INSERT INTO reviews (user_id, movie_id, text, rating, edited)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, created_at`)).
			WithArgs(review.UserID, review.MovieID, review.Text, review.Rating, review.Edited).
			WillReturnError(errors.New("database error"))

		err := m.Insert(review)
		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_Get(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Success with userID", func(t *testing.T) {
		userID := int64(2)
		query := regexp.QuoteMeta(`
        SELECT r.id, r.user_id, r.movie_id, r.text, r.rating,
               r.upvotes, r.downvotes, r.created_at, r.edited,
               u.name AS user_name,
               (r.upvotes - r.downvotes) AS total_votes,
               COALESCE(rv.vote_type, 0) AS user_vote
        FROM reviews r
        JOIN users u ON r.user_id = u.id
        LEFT JOIN review_vote rv ON rv.review_id = r.id AND rv.user_id = $2
        WHERE r.id = $1`)

		mock.ExpectQuery(query).
			WithArgs(int64(1), userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "created_at", "edited", "user_name", "total_votes", "user_vote",
			}).AddRow(1, 1, 1, "Great movie!", 8, 10, 2, fixedCreatedAt, false, "Test User", 8, 1))

		review, err := m.Get(context.Background(), 1, &userID)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), review.ID)
		assert.Equal(t, int64(1), review.UserID)
		assert.Equal(t, int64(1), review.MovieID)
		assert.Equal(t, "Great movie!", review.Text)
		assert.Equal(t, uint8(8), review.Rating)
		assert.Equal(t, int32(10), review.Upvotes)
		assert.Equal(t, int32(2), review.Downvotes)
		assert.Equal(t, false, review.Edited)
		assert.Equal(t, "Test User", review.UserName)
		assert.Equal(t, int32(8), review.TotalVotes)
		assert.Equal(t, 1, review.CurrentUserVote)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success without userID", func(t *testing.T) {
		query := regexp.QuoteMeta(`
        SELECT r.id, r.user_id, r.movie_id, r.text, r.rating,
               r.upvotes, r.downvotes, r.created_at, r.edited,
               u.name AS user_name,
               (r.upvotes - r.downvotes) AS total_votes,
               COALESCE(rv.vote_type, 0) AS user_vote
        FROM reviews r
        JOIN users u ON r.user_id = u.id
        
        WHERE r.id = $1`)

		mock.ExpectQuery(query).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "created_at", "edited", "user_name", "total_votes", "user_vote",
			}).AddRow(1, 1, 1, "Great movie!", 8, 10, 2, fixedCreatedAt, false, "Test User", 8, 0))

		review, err := m.Get(context.Background(), 1, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), review.ID)
		assert.Equal(t, "Test User", review.UserName)
		assert.Equal(t, int32(8), review.TotalVotes)
		assert.Equal(t, 0, review.CurrentUserVote)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		userID := int64(2)
		query := regexp.QuoteMeta(`
        SELECT r.id, r.user_id, r.movie_id, r.text, r.rating,
               r.upvotes, r.downvotes, r.created_at, r.edited,
               u.name AS user_name,
               (r.upvotes - r.downvotes) AS total_votes,
               COALESCE(rv.vote_type, 0) AS user_vote
        FROM reviews r
        JOIN users u ON r.user_id = u.id
        LEFT JOIN review_vote rv ON rv.review_id = r.id AND rv.user_id = $2
        WHERE r.id = $1`)

		mock.ExpectQuery(query).
			WithArgs(int64(999), userID).
			WillReturnError(sql.ErrNoRows)

		_, err := m.Get(context.Background(), 999, &userID)
		assert.ErrorIs(t, err, ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_GetFiltered(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	currentUserID := int64(2)
	rf := filters.NewReviewFilters()

	t.Run("Success", func(t *testing.T) {
		query, args := filters.NewReviewQueryBuilder().Build(rf, currentUserID)
		t.Logf("Query: %s, Args: %v", query, args)

		escapedQuery := regexp.QuoteMeta(query)
		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{
				"total_records", "id", "user_id", "movie_id", "text", "rating", "created_at",
				"upvotes", "downvotes", "edited", "user_name", "total_votes", "user_vote",
			}).AddRow(1, 1, 1, 1, "Great movie!", 8, fixedCreatedAt, 10, 2, false, "Test User", 8, 1))

		reviews, total, err := m.GetFiltered(context.Background(), currentUserID, rf)
		assert.NoError(t, err)
		assert.Len(t, reviews, 1)
		assert.Equal(t, 1, total)
		assert.Equal(t, int64(1), reviews[0].ID)
		assert.Equal(t, "Test User", reviews[0].UserName)
		assert.Equal(t, int32(8), reviews[0].TotalVotes)
		assert.Equal(t, 1, reviews[0].CurrentUserVote)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("No rows", func(t *testing.T) {
		query, args := filters.NewReviewQueryBuilder().Build(rf, currentUserID)
		escapedQuery := regexp.QuoteMeta(query)
		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{
				"total_records", "id", "user_id", "movie_id", "text", "rating", "created_at",
				"upvotes", "downvotes", "edited", "user_name", "total_votes", "user_vote",
			}))

		reviews, total, err := m.GetFiltered(context.Background(), currentUserID, rf)
		assert.NoError(t, err)
		assert.Empty(t, reviews)
		assert.Equal(t, 0, total)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Query error", func(t *testing.T) {
		query, args := filters.NewReviewQueryBuilder().Build(rf, currentUserID)
		escapedQuery := regexp.QuoteMeta(query)
		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnError(errors.New("database error"))

		reviews, total, err := m.GetFiltered(context.Background(), currentUserID, rf)
		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())
		assert.Nil(t, reviews)
		assert.Equal(t, 0, total)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_GetByUserID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Success", func(t *testing.T) {
		userID := int64(1)
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT id, user_id, movie_id, text, rating, upvotes, downvotes, edited, created_at
        FROM reviews
        WHERE user_id = $1
        ORDER BY created_at DESC`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "edited", "created_at",
			}).AddRow(1, 1, 1, "Great movie!", 8, 10, 2, false, fixedCreatedAt))

		reviews, err := m.GetByUserID(context.Background(), userID)
		assert.NoError(t, err)
		assert.Len(t, reviews, 1)
		assert.Equal(t, int64(1), reviews[0].ID)
		assert.Equal(t, "Great movie!", reviews[0].Text)
		assert.Equal(t, uint8(8), reviews[0].Rating)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid userID", func(t *testing.T) {
		_, err := m.GetByUserID(context.Background(), 0)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("No rows", func(t *testing.T) {
		userID := int64(999)
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT id, user_id, movie_id, text, rating, upvotes, downvotes, edited, created_at
        FROM reviews
        WHERE user_id = $1
        ORDER BY created_at DESC`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "edited", "created_at",
			}))

		reviews, err := m.GetByUserID(context.Background(), userID)
		assert.NoError(t, err)
		assert.Empty(t, reviews)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_GetTopMovieReviews(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Success", func(t *testing.T) {
		movieID := int64(1)
		limit := 2
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT r.id, r.user_id, r.movie_id, LEFT(r.text, 300) AS text,
               r.rating, r.upvotes, r.downvotes, r.created_at, r.edited,
               u.name AS user_name,
               (r.upvotes - r.downvotes) AS total_votes
        FROM reviews r
        JOIN users u ON r.user_id = u.id
        WHERE r.movie_id = $1 AND r.upvotes > 0
        ORDER BY r.upvotes DESC
        LIMIT $2`)).
			WithArgs(movieID, limit).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "created_at", "edited", "user_name", "total_votes",
			}).AddRow(1, 1, 1, "Great movie!", 8, 10, 2, fixedCreatedAt, false, "Test User", 8))

		reviews, err := m.GetTopMovieReviews(context.Background(), movieID, limit)
		assert.NoError(t, err)
		assert.Len(t, reviews, 1)
		assert.Equal(t, int64(1), reviews[0].ID)
		assert.Equal(t, "Great movie!", reviews[0].Text)
		assert.Equal(t, "Test User", reviews[0].UserName)
		assert.Equal(t, int32(8), reviews[0].TotalVotes)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("No rows", func(t *testing.T) {
		movieID := int64(999)
		limit := 2
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT r.id, r.user_id, r.movie_id, LEFT(r.text, 300) AS text,
               r.rating, r.upvotes, r.downvotes, r.created_at, r.edited,
               u.name AS user_name,
               (r.upvotes - r.downvotes) AS total_votes
        FROM reviews r
        JOIN users u ON r.user_id = u.id
        WHERE r.movie_id = $1 AND r.upvotes > 0
        ORDER BY r.upvotes DESC
        LIMIT $2`)).
			WithArgs(movieID, limit).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "created_at", "edited", "user_name", "total_votes",
			}))

		reviews, err := m.GetTopMovieReviews(context.Background(), movieID, limit)
		assert.NoError(t, err)
		assert.Empty(t, reviews)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	review := &Review{
		UserID:  1,
		MovieID: 1,
		Text:    "Updated review",
		Rating:  9,
		Upvotes: 15,
	}

	t.Run("Success", func(t *testing.T) {
		reviewID := int64(1)
		mock.ExpectQuery(regexp.QuoteMeta(`
        UPDATE reviews
        SET text = $1, movie_id = $2, user_id = $3, upvotes = $4, rating = $5, edited = true
        WHERE id = $6
        RETURNING id, created_at`)).
			WithArgs(review.Text, review.MovieID, review.UserID, review.Upvotes, review.Rating, reviewID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
				AddRow(reviewID, fixedCreatedAt))

		err := m.Update(context.Background(), reviewID, review)
		assert.NoError(t, err)
		assert.Equal(t, reviewID, review.ID)
		assert.Equal(t, fixedCreatedAt, review.CreatedAt)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		reviewID := int64(999)
		mock.ExpectQuery(regexp.QuoteMeta(`
        UPDATE reviews
        SET text = $1, movie_id = $2, user_id = $3, upvotes = $4, rating = $5, edited = true
        WHERE id = $6
        RETURNING id, created_at`)).
			WithArgs(review.Text, review.MovieID, review.UserID, review.Upvotes, review.Rating, reviewID).
			WillReturnError(sql.ErrNoRows)

		err := m.Update(context.Background(), reviewID, review)
		assert.ErrorIs(t, err, ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewModel_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := ReviewModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		reviewID := int64(1)
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM reviews WHERE id = $1`)).
			WithArgs(reviewID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := m.Delete(context.Background(), reviewID)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid ID", func(t *testing.T) {
		err := m.Delete(context.Background(), 0)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("Not found", func(t *testing.T) {
		reviewID := int64(999)
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM reviews WHERE id = $1`)).
			WithArgs(reviewID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := m.Delete(context.Background(), reviewID)
		assert.ErrorIs(t, err, ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
