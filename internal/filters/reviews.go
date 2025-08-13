package filters

import (
	"cinemesis/internal/utils"
	"cinemesis/internal/validator"
	"fmt"
	"net/url"
	"strings"
)

const (
	SortByDate    = "date"
	SortByRating  = "rating"
	SortByUpvotes = "upvotes"
)

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

type ReviewFilters struct {
	PageFilters
	UserID  int64 `json:"user_id,omitempty"`
	MovieID int64 `json:"movie_id,omitempty"`

	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

type ReviewQueryBuilder struct {
	*QueryBuilder
}

func (rqb *ReviewQueryBuilder) Build(filters ReviewFilters, currentUserID int64) (string, []any) {
	return rqb.BuildReviewQuery(filters, currentUserID)
}

func NewReviewQueryBuilder() *ReviewQueryBuilder {
	return &ReviewQueryBuilder{
		QueryBuilder: NewQueryBuilder(),
	}
}

func (qb *QueryBuilder) BuildReviewQuery(filters ReviewFilters, currentUserID int64) (string, []any) {
	qb.addMovieFilter(filters.MovieID)
	qb.addUserFilter(filters.UserID)

	var whereClause string
	if len(qb.conditions) > 0 {
		whereClause = "WHERE " + strings.Join(qb.conditions, " AND ")
	}

	sortClause := filters.GetSortClause()

	var joinUserVote string
	if currentUserID > 0 {
		qb.argCount++
		joinUserVote = fmt.Sprintf(`
			LEFT JOIN review_vote rv
				ON rv.review_id = r.id AND rv.user_id = $%d
		`, qb.argCount)
		qb.args = append(qb.args, currentUserID)
	}

	query := fmt.Sprintf(`
		SELECT count(*) OVER(),
		       r.id,
		       u.name as user_name,
		       r.text,
		       r.rating,
		       r.created_at,
		       r.upvotes,
		       r.downvotes,
		       r.edited,
		       (r.upvotes + r.downvotes) as total_votes,
		       COALESCE(rv.vote_type, 0) AS user_vote
		FROM review r
		JOIN users u ON r.user_id = u.id
		%s
		%s
		ORDER BY %s, r.id ASC
		LIMIT $%d OFFSET $%d`,
		joinUserVote,
		whereClause,
		sortClause,
		qb.argCount+1,
		qb.argCount+2,
	)

	args := append(qb.args, filters.limit(), filters.offset())
	return query, args
}

func (qb *QueryBuilder) addMovieFilter(movieID int64) {
	if movieID > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.movie_id = $%d", qb.argCount))
		qb.args = append(qb.args, movieID)
	}
}

func (qb *QueryBuilder) addUserFilter(userID int64) {
	if userID > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.user_id = $%d", qb.argCount))
		qb.args = append(qb.args, userID)
	}
}

func NewReviewFilters() ReviewFilters {
	return ReviewFilters{
		PageFilters: PageFilters{
			Page:     DefaultPage,
			PageSize: DefaultPageSize,
		},
		SortBy:    SortByDate,
		SortOrder: SortOrderAsc,
	}
}

func ParseReviewFiltersFromQuery(qs url.Values, v *validator.Validator) ReviewFilters {
	filters := NewReviewFilters()

	filters.Page = utils.ReadInt(qs, "page", 1, v)
	filters.PageSize = utils.ReadInt(qs, "page_size", 20, v)

	if qs.Has("rating") {
		filters.SortBy = SortByRating
	} else if qs.Has("upvotes") {
		filters.SortBy = SortByUpvotes
	} else if qs.Has("date") {
		filters.SortBy = SortByDate
	} else {
		filters.SortBy = SortByDate
	}

	if qs.Has("desc") {
		filters.SortOrder = SortOrderDesc
	} else {
		filters.SortOrder = SortOrderAsc
	}

	return filters
}

func (rf *ReviewFilters) ValidateReviewFilters(v *validator.Validator, f ReviewFilters) {
	ValidatePageFilters(v, f.PageFilters)

	validSortBy := []string{SortByDate, SortByRating, SortByUpvotes}
	v.Check(validator.PermittedValue(f.SortBy, validSortBy...), "sort_by", "invalid sort type")

	validSortOrder := []string{SortOrderAsc, SortOrderDesc}
	v.Check(validator.PermittedValue(f.SortOrder, validSortOrder...), "sort_order", "must be 'asc' or 'desc'")
}

func (rf ReviewFilters) GetSortClause() string {
	var sortColumn string

	switch rf.SortBy {
	case SortByDate:
		sortColumn = "r.created_at"
	case SortByRating:
		sortColumn = "r.rating"
	case SortByUpvotes:
		sortColumn = "r.upvotes"
	default:
		sortColumn = "r.created_at"
	}

	direction := "ASC"
	if rf.SortOrder == SortOrderDesc {
		direction = "DESC"
	}

	return fmt.Sprintf("%s %s", sortColumn, direction)
}

func (rf ReviewFilters) ToggleSortOrder() ReviewFilters {
	newFilters := rf
	if rf.SortOrder == SortOrderAsc {
		newFilters.SortOrder = SortOrderDesc
	} else {
		newFilters.SortOrder = SortOrderAsc
	}
	return newFilters
}
