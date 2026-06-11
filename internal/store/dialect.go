package store

import (
	"database/sql"
	"strconv"
)

type Migration struct {
	Version int
	SQL     string
}

type Dialect interface {
	DriverName() string
	CreateTablesSQL() []string
	Migrations() []Migration
	BaselineVersion() int
	BoolFalse() string
	ResetSequenceOnEmpty(db *sql.DB, table string)
	ImportWipe(tx *sql.Tx)
	ImportResetSequences(tx *sql.Tx)
	UpsertNodeSQL() string
	UpsertAlertHealthSQL() string
}

func rewritePlaceholders(query string, dollarStyle bool) string {
	if !dollarStyle {
		return query
	}
	buf := make([]byte, 0, len(query)+32)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			buf = append(buf, '$')
			buf = append(buf, []byte(strconv.Itoa(n))...)
		} else {
			buf = append(buf, query[i])
		}
	}
	return string(buf)
}
