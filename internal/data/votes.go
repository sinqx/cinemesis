package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type VoteType int8

const (
	NoneVote VoteType = 0
	Upvote   VoteType = 1
	Downvote VoteType = -1
)

type ReviewVote struct {
	ReviewID int64    `json:"review_id"`
	UserID   int64    `json:"user_id"`
	VoteType VoteType `json:"vote_type"`
}

func (r ReviewModel) VoteReview(ctx context.Context, reviewID, userID int64, voteType VoteType) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	currentVote, err := r.getCurrentVote(ctx, tx, reviewID, userID)
	if err != nil {
		return err
	}

	if currentVote == voteType {
		if err := r.deleteVote(ctx, tx, reviewID, userID); err != nil {
			return err
		}
	} else if err := r.updateVote(ctx, tx, reviewID, userID, currentVote, voteType); err != nil {
		return err
	}

	if err := r.updateVoteCounts(ctx, tx, reviewID, currentVote, voteType); err != nil {
		return err
	}

	return tx.Commit()
}

func (r ReviewModel) getCurrentVote(ctx context.Context, tx *sql.Tx, reviewID, userID int64) (VoteType, error) {
	var voteType VoteType
	err := tx.QueryRowContext(ctx, `
		SELECT vote_type FROM review_votes 
		WHERE review_id = $1 AND user_id = $2`,
		reviewID, userID).Scan(&voteType)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return voteType, nil
}

func (r ReviewModel) updateVote(ctx context.Context, tx *sql.Tx, reviewID, userID int64, currentVote, newVote VoteType) error {
	if newVote == 0 {
		_, err := tx.ExecContext(ctx, `
			DELETE FROM review_votes 
			WHERE review_id = $1 AND user_id = $2`,
			reviewID, userID)
		return err
	}

	if currentVote == NoneVote {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO review_votes (review_id, user_id, vote_type)
			VALUES ($1, $2, $3)`,
			reviewID, userID, newVote)
		return err
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE review_votes 
		SET vote_type = $3
		WHERE review_id = $1 AND user_id = $2`,
		reviewID, userID, newVote)
	return err
}

func (r ReviewModel) updateVoteCounts(ctx context.Context, tx *sql.Tx, reviewID int64, oldVote, newVote VoteType) error {
	upvoteDelta := r.calculateDelta(oldVote, newVote, Upvote)
	downvoteDelta := r.calculateDelta(oldVote, newVote, Downvote)

	_, err := tx.ExecContext(ctx, `
		UPDATE reviews 
		SET upvotes = GREATEST(0, upvotes + $2),
			downvotes = GREATEST(0, downvotes + $3)
		WHERE id = $1`,
		reviewID, upvoteDelta, downvoteDelta)

	return err
}

func (r ReviewModel) deleteVote(ctx context.Context, tx *sql.Tx, reviewID, userID int64) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM review_votes
		 WHERE review_id = $1 AND user_id = $2`,
		reviewID, userID)
	fmt.Println("deleted")
	return err
}

func (r ReviewModel) calculateDelta(oldVote, newVote, targetVote VoteType) int {
	delta := 0
	if oldVote == targetVote {
		delta -= 1
	}
	if newVote == targetVote {
		delta += 1
	}
	return delta
}
