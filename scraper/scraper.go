package scraper

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/go-pg/pg/v10"
	"github.com/gocolly/colly"
)

func fetchLastUpdateDateFromDB(dbConn *pg.DB) (time.Time, error) {
	var lastUpdate time.Time
	err := dbConn.Model((*db.Metadata)(nil)).ColumnExpr("MAX(last_update)").Select(&lastUpdate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to fetch last update date from database: %w", err)
	}
	return lastUpdate, nil
}

func fetchLastUpdateDateFromWeb(content string) (time.Time, error) {
	re := regexp.MustCompile(`(\d{2})\.(\d{2})\.(\d{4})\s+(\d{2}):(\d{2})`)
	matches := re.FindStringSubmatch(content)

	if len(matches) <= 0 {
		return time.Time{}, fmt.Errorf("failed to parse date")
	}

	day := matches[1]
	month := matches[2]
	year := matches[3]
	hour := matches[4]
	minute := matches[5]

	dateString := fmt.Sprintf("%s-%s-%s %s:%s", year, month, day, hour, minute)

	layout := "2006-01-02 15:04"
	parsedTime, err := time.Parse(layout, dateString)
	if err != nil {
		log.Println("Failed to parse time: %w", err)
		return time.Time{}, err
	}

	return parsedTime, nil
}

func fetchGroups(content string) ([]string, error) {
	re := regexp.MustCompile(`var query = \['(.*?)'\]`)
	matches := re.FindStringSubmatch(content)

	if len(matches) <= 1 {
		return nil, fmt.Errorf("no matches found")
	}

	arrayString := matches[1]
	arrayString = strings.TrimSpace(arrayString)

	arrayElements := strings.Split(arrayString, `','`)
	for i := range arrayElements {
		arrayElements[i] = strings.Trim(arrayElements[i], "'")
	}

	var groups []string

	re = regexp.MustCompile(`^\d{2}[а-яА-Я]+-\d+[а-я]*$`)
	for i := range arrayElements {
		if !re.MatchString(arrayElements[i]) {
			continue
		}

		groups = append(groups, strings.Trim(arrayElements[i], " "))
	}

	return groups, nil
}

func Start(dbConn *pg.DB) {
	c := colly.NewCollector()

	var latestUpdate time.Time

	c.OnHTML("html", func(e *colly.HTMLElement) {
		mainPageContent, err := e.DOM.Html()
		if err != nil {
			log.Fatalf("Error getting main page content: %v", err)
		}

		if e.Request.URL.String() == "https://www.polessu.by/ruz/?q=&f=1" {
			// groups, err := fetchGroups(mainPageContent)
			// if err != nil {
			// 	log.Fatalf("Error fetching groups: %v", err)
			// }

			// for _, group := range groups {
			// 	fmt.Println(group)
			// }
		}

		lastUpdateDateFromWeb, err := fetchLastUpdateDateFromWeb(mainPageContent)
		if err != nil {
			log.Fatalf("Error fetching last update date from web: %v", err)
		}

		if lastUpdateDateFromWeb.After(latestUpdate) {
			latestUpdate = lastUpdateDateFromWeb
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Fatalf("Request failed: %v", err)
	})

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	for _, link := range links {
		c.Visit(link)
	}

	lastUpdateDateFromDB, err := fetchLastUpdateDateFromDB(dbConn)
	if err != nil {
		log.Fatalf("Error fetching last update date from database: %v", err)
	}

	log.Println("Date db: ", lastUpdateDateFromDB)
	log.Println("Latest date from web: ", latestUpdate)
}
