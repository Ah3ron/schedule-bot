package db

import (
	"fmt"
	"log"
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

func (u Users) String() string {
	return fmt.Sprintf("Users<%d %s %v>", u.TelegramID, u.GroupName, u.IsBanned)
}

func (s Schedule) String() string {
	return fmt.Sprintf("Schedule<%d %s %s %s %s %s %s %s>", s.ID, s.GroupName, s.LessonDate, s.DayOfWeek, s.LessonTime, s.LessonName, s.Location, s.Teacher)
}

func (m Metadata) String() string {
	return fmt.Sprintf("Metadata<%d %s>", m.ID, m.LastUpdate)
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

	printTables(db)
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

		_, err := db.Model(initialMetadata).Insert()
		if err != nil {
			return fmt.Errorf("failed to insert initial metadata: %w", err)
		}
	}

	return nil
}

func printTables(db *pg.DB) {
	var tables []string
	_, err := db.Query(&tables, `SELECT tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema')`)
	if err != nil {
		log.Fatalf("Error fetching table names: %v", err)
	}

	fmt.Println("Tables in the database:")
	for _, table := range tables {
		fmt.Println(table)
	}
}
