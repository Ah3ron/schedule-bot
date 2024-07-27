package scraper

import (
	"log"
	"strings"
	"time"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/go-pg/pg/v10"
	"github.com/gocolly/colly"
)

// CheckForUpdates проверяет обновления расписания и парсит их, если они есть.
func CheckForUpdates(dbConn *pg.DB) {
	for {
		lastUpdate, err := getLastUpdate(dbConn)
		if err != nil {
			log.Printf("Failed to get last update: %v", err)
			continue
		}

		latestUpdate, err := getLatestScheduleUpdate()
		if err != nil {
			log.Printf("Failed to get latest schedule update: %v", err)
			continue
		}

		if latestUpdate.After(lastUpdate) {
			if err := parseAndSaveSchedule(dbConn, latestUpdate); err != nil {
				log.Printf("Failed to parse and save schedule: %v", err)
				continue
			}
			log.Println("Schedule updated successfully")
		} else {
			log.Println("No updates found for the schedule")
		}

		time.Sleep(1 * time.Hour) // ждем один час до следующей проверки
	}
}

// getLastUpdate получает время последнего обновления расписания из базы данных.
func getLastUpdate(dbConn *pg.DB) (time.Time, error) {
	var metadata db.Metadata
	err := dbConn.Model(&metadata).Order("last_update DESC").Limit(1).Select()
	if err != nil {
		return time.Time{}, err
	}
	return metadata.LastUpdate, nil
}

// getLatestScheduleUpdate парсит дату последнего обновления с веб-страниц.
func getLatestScheduleUpdate() (time.Time, error) {
	var latestUpdate time.Time
	var err error

	c := colly.NewCollector()

	// Коллектор для первой страницы
	c.OnHTML(".container p.small:first-of-type", func(e *colly.HTMLElement) {
		dateStr := extractDate(e.Text)
		if dateStr != "" {
			updateTime, parseErr := time.Parse("02.01.2006 15:04", dateStr)
			if parseErr == nil && updateTime.After(latestUpdate) {
				latestUpdate = updateTime
			} else if parseErr != nil {
				err = parseErr
			}
		}
	})

	urls := []string{
		"https://www.polessu.by/ruz/",
		"https://www.polessu.by/ruz/term2/",
	}

	for _, url := range urls {
		err = c.Visit(url)
		if err != nil {
			log.Printf("Failed to visit %s: %v", url, err)
		}
	}

	return latestUpdate, err
}

// extractDate извлекает дату из строки текста.
func extractDate(text string) string {
	parts := strings.Split(text, ": ")
	if len(parts) > 1 {
		dateStr := strings.TrimSpace(strings.Split(parts[1], "\n")[0])
		return dateStr
	}
	return ""
}

// parseAndSaveSchedule парсит новое расписание и сохраняет его в базу данных.
func parseAndSaveSchedule(dbConn *pg.DB, latestUpdate time.Time) error {
	// Логика парсинга нового расписания.
	// Например, запрос к API или парсинг веб-страницы.

	// Пример нового расписания
	newSchedules := []db.Schedule{
		{GroupName: "Group1", LessonDate: "2024-07-27", DayOfWeek: "Monday", LessonTime: "10:00", LessonName: "Math", Location: "Room 101", Teacher: "John Doe", Subgroup: "A"},
	}

	// Сохраняем новое расписание в базу данных.
	for _, schedule := range newSchedules {
		_, err := dbConn.Model(&schedule).Insert()
		if err != nil {
			return err
		}
	}

	// Обновляем метаданные с новейшим временем обновления.
	metadata := &db.Metadata{LastUpdate: latestUpdate}
	_, err := dbConn.Model(metadata).OnConflict("(id) DO UPDATE").Set("last_update = EXCLUDED.last_update").Insert()
	if err != nil {
		return err
	}

	return nil
}
