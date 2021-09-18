package hdb

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type Migrator struct {
	migrator.Migrator
	Dialector
}

func (m Migrator) FullDataTypeOf(field *schema.Field) clause.Expr {
	expr := m.Migrator.FullDataTypeOf(field)

	if value, ok := field.TagSettings["COMMENT"]; ok {
		expr.SQL += " COMMENT " + m.Dialector.Explain("?", value)
	}

	return expr
}

func (m Migrator) AddColumn(value interface{}, field string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		// avoid using the same name field
		f := stmt.Schema.LookUpField(field)
		if f == nil {
			return fmt.Errorf("failed to look up field with name: %s", field)
		}

		if !f.IgnoreMigration {
			return m.DB.Exec(
				"ALTER TABLE ? ADD (? ?)",
				m.CurrentTable(stmt), clause.Column{Name: f.DBName}, m.DB.Migrator().FullDataTypeOf(f),
			).Error
		}

		return nil
	})
}

func (m Migrator) AlterColumn(value interface{}, field string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if field := stmt.Schema.LookUpField(field); field != nil {
			return m.DB.Exec(
				"ALTER TABLE ? ALTER (? ?)",
				clause.Table{Name: stmt.Table},
				clause.Column{Name: field.DBName},
				m.FullDataTypeOf(field),
			).Error
		}
		return fmt.Errorf("failed to look up field with name: %s", field)
	})
}

func (m Migrator) HasTable(value interface{}) bool {
	var count int64

	m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		return m.DB.Raw("SELECT count(*) FROM sys.tables WHERE schema_name = ? AND table_name = ?", currentDatabase, stmt.Table).Row().Scan(&count)
	})

	return count > 0
}

func (m Migrator) HasColumn(value interface{}, field string) bool {
	var count int64
	m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		name := field
		if field := stmt.Schema.LookUpField(field); field != nil {
			name = field.DBName
		}

		return m.DB.Raw(
			"SELECT count(*) FROM SYS.TABLE_COLUMNS WHERE SCHEMA_NAME = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?",
			currentDatabase, stmt.Table, name,
		).Row().Scan(&count)
	})

	return count > 0
}

func (m Migrator) CurrentDatabase() (name string) {
	m.DB.Raw("SELECT CURRENT_SCHEMA FROM DUMMY").Row().Scan(&name)
	return
}

func (m Migrator) MigrateColumn(value interface{}, field *schema.Field, columnType gorm.ColumnType) error {

	alterColumn := false

	// check precision
	if precision, scale, ok := columnType.DecimalSize(); ok && field.Precision > 0 {
		if precision > 0 && precision != int64(field.Precision) {
			alterColumn = true
		}
		if scale > 0 && scale != int64(field.Scale) {
			alterColumn = true
		}
	}

	// for string or blob ?
	if length, ok := columnType.Length(); ok && length > 0 && field.Size > 0 {
		if length != int64(field.Size) {
			alterColumn = true
		}
	}

	// check nullable
	if nullable, ok := columnType.Nullable(); ok && nullable == field.NotNull {
		// not primary key & database is nullable
		if !field.PrimaryKey && nullable {
			alterColumn = true
		}
	}

	if alterColumn && !field.IgnoreMigration {
		return m.DB.Migrator().AlterColumn(value, field.Name)
	}

	return nil
}

func (m Migrator) RenameColumn(value interface{}, oldName, newName string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {

		var field *schema.Field
		if f := stmt.Schema.LookUpField(oldName); f != nil {
			oldName = f.DBName
			field = f
		}

		if f := stmt.Schema.LookUpField(newName); f != nil {
			newName = f.DBName
			field = f
		}

		if field != nil {
			return m.DB.Exec(
				"ALTER TABLE ? CHANGE ? ? ?",
				clause.Table{Name: stmt.Table}, clause.Column{Name: oldName},
				clause.Column{Name: newName}, m.FullDataTypeOf(field),
			).Error
		}

		return fmt.Errorf("failed to look up field with name: %s", newName)
	})
}

func (m Migrator) RenameIndex(value interface{}, oldName, newName string) error {

	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		err := m.DropIndex(value, oldName)
		if err != nil {
			return err
		}

		if idx := stmt.Schema.LookIndex(newName); idx == nil {
			if idx = stmt.Schema.LookIndex(oldName); idx != nil {
				opts := m.BuildIndexOptions(idx.Fields, stmt)
				values := []interface{}{clause.Column{Name: newName}, clause.Table{Name: stmt.Table}, opts}

				createIndexSQL := "CREATE "
				if idx.Class != "" {
					createIndexSQL += idx.Class + " "
				}
				createIndexSQL += "INDEX ? ON ??"

				if idx.Type != "" {
					createIndexSQL += " USING " + idx.Type
				}

				return m.DB.Exec(createIndexSQL, values...).Error
			}
		}

		return m.CreateIndex(value, newName)
	})

}

func (m Migrator) DropTable(values ...interface{}) error {
	values = m.ReorderModels(values, false)
	tx := m.DB.Session(&gorm.Session{})
	for i := len(values) - 1; i >= 0; i-- {
		if err := m.RunWithValue(values[i], func(stmt *gorm.Statement) error {
			return tx.Exec("DROP TABLE IF EXISTS ? CASCADE", clause.Table{Name: stmt.Table}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m Migrator) DropConstraint(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, chk, table := m.GuessConstraintAndTable(stmt, name)
		if chk != nil {
			return m.DB.Exec("ALTER TABLE ? DROP CHECK ?", clause.Table{Name: stmt.Table}, clause.Column{Name: chk.Name}).Error
		}
		if constraint != nil {
			name = constraint.Name
		}

		return m.DB.Exec(
			"ALTER TABLE ? DROP FOREIGN KEY ?", clause.Table{Name: table}, clause.Column{Name: name},
		).Error
	})
}

// ColumnTypes column types return columnTypes,error
func (m Migrator) ColumnTypes(value interface{}) ([]gorm.ColumnType, error) {
	columnTypes := make([]gorm.ColumnType, 0)
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		columnTypeSQL := "SELECT column_name, is_nullable, data_type_name, length, scale FROM SYS.TABLE_COLUMNS WHERE schema_name = ? AND table_name = ? ORDER BY POSITION ASC"

		columns, rowErr := m.DB.Raw(columnTypeSQL, currentDatabase, stmt.Table).Rows()
		if rowErr != nil {
			return rowErr
		}

		defer columns.Close()

		for columns.Next() {
			var column Column

			var values = []interface{}{
				&column.name,
				&column.nullable,
				&column.datatype,
				&column.length,
				&column.scale,
			}

			if scanErr := columns.Scan(values...); scanErr != nil {
				return scanErr
			}

			columnTypes = append(columnTypes, column)
		}

		return nil
	})

	return columnTypes, err
}
