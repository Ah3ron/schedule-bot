package db

import (
	"fmt"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

type Schedule struct {
	ID         int64
	GroupName  string `pg:",notnull"`
	LessonDate string `pg:",notnull"`
	DayOfWeek  string `pg:",notnull"`
	LessonTime string `pg:",notnull"`
	LessonName string `pg:",notnull"`
	Location   string
	Teacher    string
	Subgroup   string
}

type Users struct {
	TelegramID int64  `pg:",pk"`
	GroupName  string `pg:",notnull"`
	IsBanned   bool   `pg:",use_zero,default:false"`
}

type Metadata struct {
	ID         int64
	LastUpdate time.Time `pg:",notnull"`
}

func InitDB(databaseURL string) (*pg.DB, error) {
	opt, err := pg.ParseURL(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	db := pg.Connect(opt)
	if err := createSchema(db); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return db, nil
}

func createSchema(db *pg.DB) error {
	models := []interface{}{
		(*Schedule)(nil),
		(*Users)(nil),
		(*Metadata)(nil),
	}

	for _, model := range models {
		if err := db.Model(model).CreateTable(&orm.CreateTableOptions{Temp: false, IfNotExists: true}); err != nil {
			return fmt.Errorf("failed to create table for model %T: %w", model, err)
		}
	}

	var count int
	_, err := db.QueryOne(pg.Scan(&count), `SELECT COUNT(*) FROM metadata`)
	if err != nil {
		return fmt.Errorf("failed to query metadata count: %w", err)
	}

	if count == 0 {
		initialMetadata := &Metadata{LastUpdate: time.Unix(0, 0).UTC()}
		if _, err := db.Model(initialMetadata).Insert(); err != nil {
			return fmt.Errorf("failed to insert initial metadata: %w", err)
		}
	}

	return nil
}
