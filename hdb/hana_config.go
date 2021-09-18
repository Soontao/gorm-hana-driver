package hdb

import "gorm.io/gorm"

type Config struct {
	DriverName string
	DSN        string
	Conn       gorm.ConnPool
}
