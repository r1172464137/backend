package orm

import (
	"strings"

	"gorm.io/gorm"
)

const likeEscapeChar = "="

// LikeEscapeClause is the ESCAPE clause of the patterns built here. They
// escape with '=' rather than the backslash, which MySQL and PostgreSQL
// quote differently in a string literal.
func LikeEscapeClause() string {
	return " ESCAPE '" + likeEscapeChar + "'"
}

// LikePrefixPattern returns the escaped LIKE pattern matching values that
// start with value, or "" for a blank value.
func LikePrefixPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return escapeLike(value) + "%"
}

// LikeContainsPattern returns the escaped LIKE pattern matching values that
// contain value, or "" for a blank value.
func LikeContainsPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "%" + escapeLike(value) + "%"
}

// PrefixLike is a GORM scope keeping the rows where one of fields starts
// with value; a blank value keeps every row.
func PrefixLike(fields []string, value string) func(db *gorm.DB) *gorm.DB {
	return likeSearch(fields, LikePrefixPattern(value))
}

// ContainsLike is a GORM scope keeping the rows where one of fields contains
// value; a blank value keeps every row.
func ContainsLike(fields []string, value string) func(db *gorm.DB) *gorm.DB {
	return likeSearch(fields, LikeContainsPattern(value))
}

func likeSearch(fields []string, pattern string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(fields) == 0 || pattern == "" {
			return db
		}

		conds := make([]string, 0, len(fields))
		args := make([]any, 0, len(fields))
		for _, field := range fields {
			if field == "" {
				continue
			}
			conds = append(conds, field+" LIKE ?"+LikeEscapeClause())
			args = append(args, pattern)
		}
		if len(conds) == 0 {
			return db
		}
		return db.Where("("+strings.Join(conds, " OR ")+")", args...)
	}
}

// LikeEscape escapes value for use inside a LIKE pattern paired with
// LikeEscapeClause.
func LikeEscape(value string) string {
	return escapeLike(value)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(likeEscapeChar, likeEscapeChar+likeEscapeChar, `%`, likeEscapeChar+`%`, `_`, likeEscapeChar+`_`)
	return replacer.Replace(value)
}
