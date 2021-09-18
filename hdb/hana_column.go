package hdb

import (
	"database/sql"
	"strings"
)

type Column struct {
	name     string
	nullable bool
	datatype string
	length   uint
	scale    sql.NullInt64
}

func (c Column) Name() string {
	return c.name
}

func (c Column) DatabaseTypeName() (datatype string) {
	return strings.ToLower(c.datatype)
}

// character size
func (c Column) Length() (int64, bool) {
	if isNumericDataType(c.DatabaseTypeName()) {
		return 0, false
	}
	return int64(c.length), true
}

func (c Column) Nullable() (bool, bool) {
	return c.nullable, true
}

// DecimalSize return precision int64, scale int64, ok bool
func (c Column) DecimalSize() (precision int64, scale int64, ok bool) {
	if isNumericDataType(c.DatabaseTypeName()) {
		precision = int64(c.length)
		if c.scale.Valid {
			scale = c.scale.Int64
		}
		ok = true
	}
	return
}
