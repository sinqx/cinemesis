package filters

import (
	"cinemesis/internal/utils"
	"cinemesis/internal/validator"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"
)

type MovieFilters struct {
	PageFilters
	Title      string   `json:"title,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	MinYear    int32    `json:"min_year,omitempty"`
	MaxYear    int32    `json:"max_year,omitempty"`
	MinRuntime int32    `json:"min_runtime,omitempty"`
	MaxRuntime int32    `json:"max_runtime,omitempty"`
}

func  NewMovieFilters() MovieFilters {
	return MovieFilters{
		PageFilters: PageFilters{
			Page:     1,
			PageSize: 20,
			Sort:     "id",
			SortSafelist: []string{
				"id", "title", "year", "runtime",
				"-id", "-title", "-year", "-runtime",
			},
		},
	}
}

func ParseMovieFiltersFromQuery(qs url.Values, v *validator.Validator) MovieFilters {
	filters := NewMovieFilters()

	filters.Page = utils.ReadInt(qs, "page", 1, v)
	filters.PageSize = utils.ReadInt(qs, "page_size", 20, v)
	filters.Sort = utils.ReadString(qs, "sort", "id")
	filters.Title = utils.ReadString(qs, "title", "")
	filters.Genres = utils.ReadCSV(qs, "genres", []string{})
	filters.MinYear = int32(utils.ReadInt(qs, "min_year", 0, v))
	filters.MaxYear = int32(utils.ReadInt(qs, "max_year", 0, v))
	filters.MinRuntime = int32(utils.ReadInt(qs, "min_runtime", 0, v))
	filters.MaxRuntime = int32(utils.ReadInt(qs, "max_runtime", 0, v))

	return filters
}

func (mf *MovieFilters) ValidateMovieFilters(v *validator.Validator, f MovieFilters) {
	ValidatePageFilters(v, f.PageFilters)

	v.Check(f.MinYear == 0 || f.MinYear >= 1888, "min_year", "must be greater than 1888")
	v.Check(f.MaxYear == 0 || f.MaxYear <= int32(time.Now().Year()+10), "max_year", "must not be too far in the future")
	v.Check(f.MinYear == 0 || f.MaxYear == 0 || f.MinYear <= f.MaxYear, "max_year", "must be greater than min_year")

	v.Check(f.MinRuntime == 0 || f.MinRuntime > 0, "min_runtime", "must be greater than zero")
	v.Check(f.MaxRuntime == 0 || f.MaxRuntime <= 1000, "max_runtime", "must be a maximum of 1000 minutes")
	v.Check(f.MinRuntime == 0 || f.MaxRuntime == 0 || f.MinRuntime <= f.MaxRuntime, "max_runtime", "must be greater than min_runtime")
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

func (qb *QueryBuilder) AddYearRangeFilter(minYear, maxYear int32) *QueryBuilder {
	if minYear > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("m.year >= $%d", qb.argCount))
		qb.args = append(qb.args, minYear)
	}
	if maxYear > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("m.year <= $%d", qb.argCount))
		qb.args = append(qb.args, maxYear)
	}
	return qb
}

func (qb *QueryBuilder) AddRuntimeRangeFilter(minRuntime, maxRuntime int32) *QueryBuilder {
	if minRuntime > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("m.runtime >= $%d", qb.argCount))
		qb.args = append(qb.args, minRuntime)
	}
	if maxRuntime > 0 {
		qb.argCount++
		qb.conditions = append(qb.conditions, fmt.Sprintf("m.runtime <= $%d", qb.argCount))
		qb.args = append(qb.args, maxRuntime)
	}
	return qb
}

func (qb *QueryBuilder) BuildMovieQuery(filters MovieFilters) (string, []any) {
	var whereClause string
	if len(qb.conditions) > 0 {
		whereClause = "WHERE " + strings.Join(qb.conditions, " AND ")
	}

	columnMap := map[string]string{
		"id":      "m.id",
		"title":   "m.title",
		"year":    "m.year",
		"runtime": "m.runtime",
	}

	sortColumn := filters.sortColumn()
	actualColumn, exists := columnMap[sortColumn]
	if !exists {
		actualColumn = "m.id"
	}

	query := fmt.Sprintf(`
		SELECT count(*) OVER(), m.id, m.created_at, m.updated_at, m.title, m.year, m.runtime, m.version
		FROM movies m
		%s
		ORDER BY %s %s, m.id ASC
		LIMIT $%d OFFSET $%d`,
		whereClause,
		actualColumn,
		filters.sortDirection(),
		qb.argCount+1,
		qb.argCount+2,
	)

	args := append(qb.args, filters.limit(), filters.offset())
	return query, args
}

type MovieQueryBuilder struct {
	*QueryBuilder
}

func (mqb *MovieQueryBuilder) Build(filters MovieFilters) (string, []any) {
	return mqb.BuildMovieQuery(filters)
}

func NewMovieQueryBuilder() *MovieQueryBuilder {
	return &MovieQueryBuilder{
		QueryBuilder: NewQueryBuilder(),
	}
}

func (mqb *MovieQueryBuilder) WithTitle(title string) *MovieQueryBuilder {
	if title != "" {
		mqb.AddTitleFilter(title)
	}
	return mqb
}

func (mqb *MovieQueryBuilder) WithGenres(genreIDs []int64) *MovieQueryBuilder {
	if len(genreIDs) > 0 {
		mqb.AddGenreFilter(genreIDs)
	}
	return mqb
}

func (mqb *MovieQueryBuilder) WithYearRange(min, max int32) *MovieQueryBuilder {
	mqb.AddYearRangeFilter(min, max)
	return mqb
}

func (mqb *MovieQueryBuilder) WithRuntimeRange(min, max int32) *MovieQueryBuilder {
	mqb.AddRuntimeRangeFilter(min, max)
	return mqb
}
