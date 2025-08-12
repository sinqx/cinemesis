package filters

import (
	"cinemesis/internal/utils"
	"cinemesis/internal/validator"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	SortByDate       = "date"
	SortByRating     = "rating"
	SortByPopular    = "popular"
	SortByUnpopular  = "unpopular"
	SortByTotalVotes = "total_votes"
)

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

type ReviewFilters struct {
	PageFilters
	MinRating  uint8     `json:"min_rating,omitempty"`
	MaxRating  uint8     `json:"max_rating,omitempty"`
	MinUpvotes int32     `json:"min_upvotes,omitempty"`
	DateFrom   time.Time `json:"date_from,omitempty"`
	DateTo     time.Time `json:"date_to,omitempty"`
	UserID     int64     `json:"user_id,omitempty"`
	MovieID    int64     `json:"movie_id,omitempty"`

	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

type ReviewQueryBuilder struct {
	*QueryBuilder
}

func (rqb *QueryBuilder) Build(filters ReviewFilters) (string, []any) {
	return rqb.BuildReviewQuery(filters)
}

func NewReviewQueryBuilder() *ReviewQueryBuilder {
	return &ReviewQueryBuilder{
		QueryBuilder: NewQueryBuilder(),
	}
}
func (qb *QueryBuilder) BuildReviewQuery(filters ReviewFilters) (string, []any) {
	var whereClause string
	if len(qb.conditions) > 0 {
		whereClause = "WHERE " + strings.Join(qb.conditions, " AND ")
	}

	sortClause := filters.GetSortClause()

	var joinUserVote string
	if filters.UserID > 0 {
		qb.argCount++
		joinUserVote = fmt.Sprintf(`
			LEFT JOIN review_vote rv
				ON rv.review_id = r.id AND rv.user_id = $%d
		`, qb.argCount)
		qb.args = append(qb.args, filters.UserID)
	}

	query := fmt.Sprintf(`
		SELECT count(*) OVER(),
		       r.id,
		       u.name as user_name,
		       r.text,
		       r.rating
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

func NewReviewFilters() ReviewFilters {
	return ReviewFilters{
		PageFilters: PageFilters{
			Page:     DefaultPage,
			PageSize: DefaultPageSize,
			SortSafelist: []string{
				"id", "rating", "upvotes", "date_from",
				"-id", "-rating", "-upvotes", "date_to",
			},
		},
		SortBy:    SortByDate,
		SortOrder: SortOrderDesc,
	}
}

func ParseReviewFiltersFromQuery(qs url.Values, v *validator.Validator) ReviewFilters {
	filters := NewReviewFilters()

	filters.Page = utils.ReadInt(qs, "page", 1, v)
	filters.PageSize = utils.ReadInt(qs, "page_size", 20, v)
	filters.SortBy = utils.ReadString(qs, "sort_by", SortByDate)
	filters.SortOrder = utils.ReadString(qs, "sort_order", SortOrderDesc)

	filters.MinRating = uint8(utils.ReadInt(qs, "min_rating", 0, v))
	filters.MaxRating = uint8(utils.ReadInt(qs, "max_rating", 0, v))
	filters.MinUpvotes = int32(utils.ReadInt(qs, "max_upvotes", 0, v))
	filters.MinUpvotes = int32(utils.ReadInt(qs, "min_upvotes", 0, v))
	filters.UserID = int64(utils.ReadInt(qs, "user_id", 0, v))

	if dateFrom := utils.ReadString(qs, "date_from", ""); dateFrom != "" {
		if parsedDate, err := time.Parse("2006-01-02", dateFrom); err == nil {
			filters.DateFrom = parsedDate
		}
	}
	if dateTo := utils.ReadString(qs, "date_to", ""); dateTo != "" {
		if parsedDate, err := time.Parse("2006-01-02", dateTo); err == nil {
			filters.DateTo = parsedDate
		}
	}

	return filters
}

func (rf *ReviewFilters) ValidateReviewFilters(v *validator.Validator, f ReviewFilters) {
	ValidatePageFilters(v, f.PageFilters)

	validSortBy := []string{SortByDate, SortByRating, SortByPopular, SortByUnpopular, SortByTotalVotes}
	v.Check(validator.PermittedValue(f.SortBy, validSortBy...), "sort_by", "invalid sort type")

	validSortOrder := []string{SortOrderAsc, SortOrderDesc}
	v.Check(validator.PermittedValue(f.SortOrder, validSortOrder...), "sort_order", "must be 'asc' or 'desc'")

	v.Check(f.MinRating >= 1, "min_rating", "must be minimum 1")
	v.Check(f.MaxRating <= 10, "max_rating", "must be maximum 10")
	v.Check(f.MinRating <= f.MaxRating || f.MaxRating == 0, "max_rating", "must be greater than min_rating")
	v.Check(f.MinUpvotes >= 0, "min_upvotes", "must be greater than or equal to zero")
	v.Check(f.UserID >= 0, "user_id", "must be greater than or equal to zero")
}

func (qb *QueryBuilder) AddRatingFilter(minRating, maxRating uint8) *QueryBuilder {
	if minRating > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.rating >= $%d", qb.argCount))
		qb.args = append(qb.args, minRating)
	}
	if maxRating > 0 && maxRating <= 10 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.rating <= $%d", qb.argCount))
		qb.args = append(qb.args, maxRating)
	}
	return qb
}

func (qb *QueryBuilder) AddUpvotesFilter(minUpvotes int32) *QueryBuilder {
	if minUpvotes > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.upvotes >= $%d", qb.argCount))
		qb.args = append(qb.args, minUpvotes)
	}
	return qb
}

func (qb *QueryBuilder) AddDateRangeFilter(from, to time.Time) *QueryBuilder {
	if !from.IsZero() {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.created_at >= $%d", qb.argCount))
		qb.args = append(qb.args, from)
	}
	if !to.IsZero() {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.created_at <= $%d", qb.argCount))
		qb.args = append(qb.args, to)
	}
	return qb
}

func (qb *QueryBuilder) AddUserFilter(userID int64) *QueryBuilder {
	if userID > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.user_id = $%d", qb.argCount))
		qb.args = append(qb.args, userID)
	}
	return qb
}

func (qb *QueryBuilder) AddMovieFilter(movieID int64) *QueryBuilder {
	if movieID > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("r.movie_id = $%d", qb.argCount))
		qb.args = append(qb.args, movieID)
	}
	return qb
}

func (rqb *ReviewQueryBuilder) WithMovieID(movieID int64) *ReviewQueryBuilder {
	rqb.AddMovieFilter(movieID)
	return rqb
}

func (rqb *ReviewQueryBuilder) WithRatingRange(min, max uint8) *ReviewQueryBuilder {
	rqb.AddRatingFilter(min, max)
	return rqb
}

func (rqb *ReviewQueryBuilder) WithMinUpvotes(minUpvotes int32) *ReviewQueryBuilder {
	rqb.AddUpvotesFilter(minUpvotes)
	return rqb
}

func (rqb *ReviewQueryBuilder) WithDateRange(from, to time.Time) *ReviewQueryBuilder {
	rqb.AddDateRangeFilter(from, to)
	return rqb
}

func (rqb *ReviewQueryBuilder) WithUser(userID int64) *ReviewQueryBuilder {
	rqb.AddUserFilter(userID)
	return rqb
}

func (rf ReviewFilters) GetSortClause() string {
	var sortColumn string

	switch rf.SortBy {
	case SortByDate:
		sortColumn = "r.created_at"
	case SortByRating:
		sortColumn = "r.rating"
	case SortByPopular:
		sortColumn = "r.upvotes"
	case SortByUnpopular:
		sortColumn = "r.downvotes"
	case SortByTotalVotes:
		sortColumn = "(r.upvotes + r.downvotes)"
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
