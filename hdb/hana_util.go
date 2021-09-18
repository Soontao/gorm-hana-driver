package hdb

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

var hanaFixedLenNumericTypes = []string{
	"double",
	"float",
	"tinyint",
	"smallint",
	"bigint",
	"integer",
	"real",
}

var hanaDynamicPrecisionNumericTypes = []string{
	"decimal",
	"smalldecimal",
}

var hanaNumericTypes = append(hanaFixedLenNumericTypes, hanaDynamicPrecisionNumericTypes...)

func isNumericDataType(datatypeName string) bool {
	lDataTypeName := strings.ToLower(datatypeName)
	for _, aType := range hanaNumericTypes {
		if strings.HasPrefix(lDataTypeName, aType) {
			return true
		}
	}
	return false
}

func RegisterCallbacks(db *gorm.DB) {
	db.Callback().Create().Replace("gorm:create", hanaCreateCallback)
}

func hanaCreateCallback(db *gorm.DB) {
	if db.Error != nil {
		return
	}

	if db.Statement.Schema != nil && !db.Statement.Unscoped {
		for _, c := range db.Statement.Schema.CreateClauses {
			db.Statement.AddClause(c)
		}
	}

	if db.Statement.SQL.String() == "" {
		db.Statement.SQL.Grow(180)
		db.Statement.AddClauseIfNotExists(clause.Insert{})
		db.Statement.AddClause(callbacks.ConvertToCreateValues(db.Statement))

		db.Statement.Build(db.Statement.BuildClauses...)
	}

	if !db.DryRun && db.Error == nil {
		result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)

		if err != nil {
			db.AddError(err)
			return
		}

		db.RowsAffected, _ = result.RowsAffected()

	}
}
