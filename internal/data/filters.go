package data

import (
	"cinemesis/internal/validator"
	"fmt"
	"slices"
	"strings"

	"github.com/lib/pq"
)

type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafelist []string
}

type Metadata struct {
	CurrentPage  int `json:"current_page,omitzero"`
	PageSize     int `json:"page_size,omitzero"`
	FirstPage    int `json:"first_page,omitzero"`
	LastPage     int `json:"last_page,omitzero"`
	TotalRecords int `json:"total_records,omitzero"`
}

func ValidateFilters(v *validator.Validator, f Filters) {
	v.Check(f.Page > 0, "page", "must be greater than zero")
	v.Check(f.Page <= 10_000_000, "page", "must be a maximum of 10 million")
	v.Check(f.PageSize > 0, "page_size", "must be greater than zero")
	v.Check(f.PageSize <= 100, "page_size", "must be a maximum of 100")
	v.Check(validator.PermittedValue(f.Sort, f.SortSafelist...), "sort", "invalid sort value")
}

func (f Filters) sortColumn() string {
	if slices.Contains(f.SortSafelist, f.Sort) {
		return strings.TrimPrefix(f.Sort, "-")
	}
	panic("unsafe sort parameter: " + f.Sort)
}

func (f Filters) sortDirection() string {
	if strings.HasPrefix(f.Sort, "-") {
		return "DESC"
	}
	return "ASC"
}

func (f Filters) limit() int {
	return f.PageSize
}
func (f Filters) offset() int {
	return (f.Page - 1) * f.PageSize
}

func calculateMetadata(totalRecords, page, pageSize int) Metadata {
	if totalRecords == 0 {
		return Metadata{}
	}
	return Metadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		FirstPage:    1,
		LastPage:     (totalRecords + pageSize - 1) / pageSize,
		TotalRecords: totalRecords,
	}
}

type QueryBuilder struct {
	conditions []string
	args       []any
	argCount   int
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		conditions: make([]string, 0),
		args:       make([]any, 0),
		argCount:   0,
	}
}

func (qb *QueryBuilder) AddTitleFilter(title string) *QueryBuilder {
	qb.argCount++
	qb.conditions = append(qb.conditions,
		fmt.Sprintf("(to_tsvector('simple', m.title) @@ plainto_tsquery('simple', $%d) OR $%d = '')",
			qb.argCount, qb.argCount))
	qb.args = append(qb.args, title)
	return qb
}

func (qb *QueryBuilder) AddGenreFilter(genreIDs []int64) *QueryBuilder {
	if len(genreIDs) == 0 {
		return qb
	}

	qb.argCount++
	arrayArg := qb.argCount
	qb.argCount++
	countArg := qb.argCount

	qb.conditions = append(qb.conditions, fmt.Sprintf(`
		m.id IN (
			SELECT movie_id FROM movies_genres
			WHERE genre_id = ANY($%d)
			GROUP BY movie_id
			HAVING COUNT(DISTINCT genre_id) = $%d
		)`, arrayArg, countArg))

	qb.args = append(qb.args, pq.Array(genreIDs), len(genreIDs))
	return qb
}

func (qb *QueryBuilder) Build(filters Filters) (string, []any) {
	whereClause := strings.Join(qb.conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT count(*) OVER(), m.id, m.created_at, m.updated_at, m.title, m.year, m.runtime, m.version
		FROM movies m
		WHERE %s
		ORDER BY %s %s, m.id ASC
		LIMIT $%d OFFSET $%d`,
		whereClause,
		filters.sortColumn(),
		filters.sortDirection(),
		qb.argCount+1,
		qb.argCount+2,
	)

	args := append(qb.args, filters.limit(), filters.offset())
	return query, args
}
