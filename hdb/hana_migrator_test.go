package hdb

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMigration(t *testing.T) {
	dsn := os.Getenv("GORM_TEST_DSN")
	if len(dsn) > 0 {
		type Peoples struct {
			UUID   string `gorm:"primaryKey;size:36"`
			Name   string
			Email  *string
			Age    uint
			Weight float32
		}
		assert := assert.New(t)
		db, err := gorm.Open(New(Config{
			DriverName: "hdb",
			DSN:        dsn,
		}))

		assert.Nil(err)
		RegisterCallbacks(db)

		// sync db
		err = db.AutoMigrate(&Peoples{})
		assert.Nil(err)

		// crud
		id := uuid.New().String()
		// create record
		result := db.Create(&Peoples{UUID: id, Name: "Theo Test"})
		assert.Nil(result.Error)

		// update and verify
		result = db.Model(&Peoples{}).Where(&Peoples{UUID: id}).Update("age", 19)
		assert.Nil(result.Error)
		query := &Peoples{}
		result = db.Where(&Peoples{UUID: id}).First(query)
		assert.Nil(result.Error)
		assert.Equal(uint(19), query.Age)

		// delete and verify
		result = db.Delete(&Peoples{UUID: id})
		assert.Nil(result.Error)
		query2 := &Peoples{}
		result = db.Where(&Peoples{UUID: id}).First(query2)
		assert.Zero(result.RowsAffected)
	}

}
