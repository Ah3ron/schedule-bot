package main

import (
	"log"
	"os"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/Ah3ron/schedule-bot/scraper"
	"github.com/Ah3ron/schedule-bot/telegram_bot"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	dbConn, err := db.InitDB(databaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbConn.Close()

	go scraper.Start(dbConn)
	go telegram_bot.Start(os.Getenv("TELEGRAM_TOKEN"), dbConn)

	select {}
}
