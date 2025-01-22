package scraper

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/go-pg/pg/v10"
	"github.com/gocolly/colly"
)

var (
	dateRegex  = regexp.MustCompile(`(\d{2})\.(\d{2})\.(\d{4})\s+(\d{2}):(\d{2})`)
	groupRegex = regexp.MustCompile(`var query = \['(.*?)'\]`)
)

func fetchLastUpdateDateFromDB(dbConn *pg.DB) (time.Time, error) {
	var lastUpdate time.Time
	err := dbConn.Model((*db.Metadata)(nil)).ColumnExpr("MAX(last_update)").Select(&lastUpdate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to fetch last update date: %w", err)
	}
	return lastUpdate, nil
}

func fetchLastUpdateDateFromWeb(content string) (time.Time, error) {
	matches := dateRegex.FindStringSubmatch(content)
	if len(matches) == 0 {
		return time.Time{}, fmt.Errorf("no date found in content")
	}

	dateStr := fmt.Sprintf("%s-%s-%s %s:%s", matches[3], matches[2], matches[1], matches[4], matches[5])
	parsedTime, err := time.Parse("2006-01-02 15:04", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse date: %w", err)
	}

	return parsedTime, nil
}

func fetchGroups(content string) ([]string, error) {
	matches := groupRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil, fmt.Errorf("no groups found in content")
	}

	groups := strings.Split(strings.TrimSpace(matches[1]), `','`)
	var validGroups []string
	for _, group := range groups {
		group = strings.Trim(group, "'")
		if regexp.MustCompile(`^\d{2}[а-яА-Я]+-\d+[а-я]*$`).MatchString(group) {
			validGroups = append(validGroups, group)
		}
	}

	return validGroups, nil
}

func parseScheduleForGroup(group string, schedChan chan<- db.Schedule, wg *sync.WaitGroup) {
	defer wg.Done()

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=1",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	for _, link := range links {
		c := colly.NewCollector()
		c.SetRequestTimeout(60 * time.Second)

		weekStartDates := parseWeekStartDates(link)

		c.OnHTML("tbody#weeks-filter", func(e *colly.HTMLElement) {
			currentDay := ""
			e.ForEach("tr", func(_ int, el *colly.HTMLElement) {
				if el.DOM.HasClass("wa") {
					currentDay = el.ChildText("th:first-of-type")
					return
				}

				weekClass := el.Attr("class")
				matches := regexp.MustCompile(`w\d+`).FindAllString(weekClass, -1)
				if len(matches) == 0 {
					return
				}

				timeRange := el.ChildText("td:nth-child(1)")
				subjectInfo := regexp.MustCompile(`\([\d\s,-]+\)\s?`).ReplaceAllString(el.ChildText("td:nth-child(2)"), "")
				room := el.ChildText("td:nth-child(3)")
				teacher := el.ChildText("td:nth-child(4)")
				subgroup := el.ChildText("td:nth-child(5) span")

				for _, weekNumber := range matches {
					startDate, ok := weekStartDates[weekNumber]
					if !ok {
						continue
					}

					classDate := startDate.AddDate(0, 0, calculateDayOfWeek(currentDay)-1)
					schedChan <- db.Schedule{
						GroupName:  group,
						LessonDate: classDate.Format("02.01"),
						DayOfWeek:  currentDay,
						LessonTime: timeRange,
						LessonName: subjectInfo,
						Location:   room,
						Teacher:    teacher,
						Subgroup:   subgroup,
					}
				}
			})
		})

		if err := c.Visit(link + "&q=" + group); err != nil {
			fmt.Printf("Failed to visit link %s: %v\n", link, err)
		}
	}
}

func saveSchedulesToDB(dbConn *pg.DB, schedules []db.Schedule, lastUpdate time.Time) error {
	if len(schedules) == 0 {
		return nil
	}

	if _, err := dbConn.Model((*db.Schedule)(nil)).Where("TRUE").Delete(); err != nil {
		return fmt.Errorf("failed to delete schedules: %w", err)
	}

	if _, err := dbConn.Model(&schedules).Insert(); err != nil {
		return fmt.Errorf("failed to insert schedules: %w", err)
	}

	if _, err := dbConn.Model(&db.Metadata{LastUpdate: lastUpdate}).Insert(); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

func calculateDayOfWeek(day string) int {
	dayMap := map[string]int{
		"Понедельник": 1,
		"Вторник":     2,
		"Среда":       3,
		"Четверг":     4,
		"Пятница":     5,
		"Суббота":     6,
		"Воскресенье": 7,
	}
	return dayMap[day]
}

func parseWeekStartDates(link string) map[string]time.Time {
	c := colly.NewCollector()
	c.SetRequestTimeout(60 * time.Second)

	weekStartDates := make(map[string]time.Time)
	var wg sync.WaitGroup

	c.OnHTML("ul#weeks-menu li a", func(e *colly.HTMLElement) {
		wg.Add(1)
		defer wg.Done()

		weekID := strings.TrimPrefix(e.Attr("href"), "#")
		if weekID == "" {
			return
		}

		dateStr := regexp.MustCompile(`\d{2}\.\d{2}`).FindString(e.Text) + fmt.Sprintf(".%d", time.Now().Year())
		startDate, err := time.Parse("02.01.2006", dateStr)
		if err != nil {
			fmt.Printf("Error parsing date for week %s: %v\n", weekID, err)
			return
		}

		weekStartDates[weekID] = startDate
	})

	c.AllowURLRevisit = true
	if err := c.Visit(link); err != nil {
		fmt.Printf("Failed to visit link %s: %v\n", link, err)
	}

	wg.Wait()
	return weekStartDates
}

func Start(dbConn *pg.DB) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := scrapeAndUpdate(dbConn); err != nil {
			fmt.Printf("Error during scraping: %v\n", err)
		}
	}
}

func scrapeAndUpdate(dbConn *pg.DB) error {
	c := colly.NewCollector()
	c.SetRequestTimeout(60 * time.Second)

	var groups []string
	var latestUpdate time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup

	c.OnHTML("html", func(e *colly.HTMLElement) {
		defer wg.Done()

		content, err := e.DOM.Html()
		if err != nil {
			fmt.Printf("Failed to get HTML content: %v\n", err)
			return
		}

		lastUpdateDateFromWeb, err := fetchLastUpdateDateFromWeb(content)
		if err != nil {
			fmt.Printf("Failed to fetch last update date: %v\n", err)
			return
		}

		mu.Lock()
		if lastUpdateDateFromWeb.After(latestUpdate) {
			latestUpdate = lastUpdateDateFromWeb
		}
		mu.Unlock()

		if e.Request.URL.String() == "https://www.polessu.by/ruz/?q=&f=1" {
			fetchedGroups, err := fetchGroups(content)
			if err != nil {
				fmt.Printf("Failed to fetch groups: %v\n", err)
				return
			}
			groups = fetchedGroups
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		fmt.Printf("Request failed: %v\n", err)
	})

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=1",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	for _, link := range links {
		wg.Add(1)
		go func(link string) {
			defer wg.Done()
			if err := c.Visit(link); err != nil {
				fmt.Printf("Failed to visit link %s: %v\n", link, err)
			}
		}(link)
	}

	wg.Wait()

	lastUpdateDateFromDB, err := fetchLastUpdateDateFromDB(dbConn)
	if err != nil {
		return fmt.Errorf("failed to fetch last update date from DB: %w", err)
	}

	if latestUpdate.After(lastUpdateDateFromDB) {
		schedChan := make(chan db.Schedule, 1000)
		var schedules []db.Schedule

		var wg sync.WaitGroup
		done := make(chan bool)

		go func() {
			for schedule := range schedChan {
				schedules = append(schedules, schedule)
			}
			done <- true
		}()

		for _, group := range groups {
			wg.Add(1)
			go parseScheduleForGroup(group, schedChan, &wg)
		}

		wg.Wait()
		close(schedChan)
		<-done

		if err := saveSchedulesToDB(dbConn, schedules, latestUpdate); err != nil {
			return fmt.Errorf("failed to save schedules: %w", err)
		}
	}

	return nil
}
