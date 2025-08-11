package filters

import (
	"cinemesis/internal/validator"
	"slices"
	"strings"
)

const (
	DefaultPage         = 1
	DefaultPageSize     = 20
	DefaultSort         = "-created_at"
	DefaultSortSafelist = ""
)

type PageFilters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafelist []string
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

func ValidatePageFilters(v *validator.Validator, f PageFilters) {
	v.Check(f.Page > 0, "page", "must be greater than zero")
	v.Check(f.Page <= 10_000_000, "page", "must be a maximum of 10 million")
	v.Check(f.PageSize > 0, "page_size", "must be greater than zero")
	v.Check(f.PageSize <= 100, "page_size", "must be a maximum of 100")
	v.Check(validator.PermittedValue(f.Sort, f.SortSafelist...), "sort", "invalid sort value")
}

func (p PageFilters) sortColumn() string {
	if slices.Contains(p.SortSafelist, p.Sort) {
		return strings.TrimPrefix(p.Sort, "-")
	}
	panic("unsafe sort parameter: " + p.Sort)
}

func (p PageFilters) sortDirection() string {
	if strings.HasPrefix(p.Sort, "-") {
		return "DESC"
	}
	return "ASC"
}

func (q QueryBuilder) AddArg(arg any) QueryBuilder {
	q.argCount += 1
	q.args = append(q.args, arg)
	return q
}

func (p PageFilters) limit() int {
	return p.PageSize
}
func (p PageFilters) offset() int {
	return (p.Page - 1) * p.PageSize
}
