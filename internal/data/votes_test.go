package data

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)


func TestCalculateDelta(t *testing.T) {
	model := ReviewModel{}

	tests := []struct {
		name     string
		oldVote  VoteType
		newVote  VoteType
		target   VoteType
		expected int
	}{
		{"upvote to same upvote", Upvote, Upvote, Upvote, 0},
		{"none to upvote", NoneVote, Upvote, Upvote, 1},
		{"upvote to none", Upvote, NoneVote, Upvote, -1},
		{"downvote to upvote", Downvote, Upvote, Upvote, 1},
		{"upvote to downvote", Upvote, Downvote, Upvote, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := model.calculateDelta(tc.oldVote, tc.newVote, tc.target)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestGetCurrentVote_NoRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	model := ReviewModel{DB: db}

	rows := sqlmock.NewRows([]string{"vote_type"})
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT vote_type FROM review_votes").
		WithArgs(int64(1), int64(2)).
		WillReturnRows(rows)
	mock.ExpectRollback()

	tx, _ := db.Begin()
	vote, err := model.getCurrentVote(context.Background(), tx, 1, 2)

	assert.NoError(t, err)
	assert.Equal(t, NoneVote, vote)
}

func TestUpdateVote_Insert(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	model := ReviewModel{DB: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO review_votes").
		WithArgs(int64(1), int64(2), Upvote).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()

	tx, _ := db.Begin()
	err := model.updateVote(context.Background(), tx, 1, 2, NoneVote, Upvote)
	assert.NoError(t, err)
}
