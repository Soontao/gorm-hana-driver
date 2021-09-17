package hdb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMigration(t *testing.T) {
	dsn := os.Getenv("test_dsn")
	if len(dsn) > 0 {
		type User struct {
			ID    uint
			Name  string
			Email *string
			Age   uint
		}
		assert := assert.New(t)
		db, err := gorm.Open(New(Config{
			DriverName: "hdb",
			DSN:        dsn,
		}))
		assert.Nil(err)
		err = db.AutoMigrate(&User{})
		assert.Nil(err)
	}

}
