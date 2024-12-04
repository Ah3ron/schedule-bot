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
		return time.Time{}, fmt.Errorf("failed to parse date from content")
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
		return time.Time{}, fmt.Errorf("failed to parse time: %w", err)
	}

	return parsedTime, nil
}

func fetchGroups(content string) ([]string, error) {
	re := regexp.MustCompile(`var query = \['(.*?)'\]`)
	matches := re.FindStringSubmatch(content)

	if len(matches) <= 1 {
		return nil, fmt.Errorf("no matches found for groups")
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

func parseScheduleForGroup(group string, schedChan chan<- db.Schedule) {
	fmt.Printf("Parsing schedule for group: %s\n", group)

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=1",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	count := 0

	for _, link := range links {
		link = link + "&q=" + group

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
				re := regexp.MustCompile(`w\d+`)
				matches := re.FindAllString(weekClass, -1)

				if len(matches) < 1 {
					return
				}

				timeRange := el.ChildText("td:nth-child(1)")
				subjectInfo := el.ChildText("td:nth-child(2)")
				room := el.ChildText("td:nth-child(3)")
				teacher := el.ChildText("td:nth-child(4)")
				subgroup := el.ChildText("td:nth-child(5) span")

				reSubjectInfo := regexp.MustCompile(`\([\d\s,-]+\)\s?`)
				subjectInfo = reSubjectInfo.ReplaceAllString(subjectInfo, "")

				dayOfWeek := calculateDayOfWeek(currentDay)

				for _, weekNumber := range matches {
					startDate, ok := weekStartDates[weekNumber]
					if !ok {
						fmt.Printf("No start date for week: %s\n", weekNumber)
						fmt.Println(group)
						continue
					}

					classDate := startDate.AddDate(0, 0, dayOfWeek-1)

					schedule := db.Schedule{
						GroupName:  group,
						LessonDate: classDate.Format("02.01"),
						DayOfWeek:  currentDay,
						LessonTime: timeRange,
						LessonName: subjectInfo,
						Location:   room,
						Teacher:    teacher,
						Subgroup:   subgroup,
					}
					count++

					schedChan <- schedule
				}
			})
		})

		if err := c.Visit(link); err != nil {
			fmt.Printf("Failed to visit link %s: %v\n", link, err)
		}
	}

	fmt.Printf("Total schedules collected from group %s: %d\n", group, count)
}

func saveSchedulesToDB(dbConn *pg.DB, schedules []db.Schedule, lastUpdate time.Time) error {
	if len(schedules) == 0 {
		fmt.Println("No schedules to save to the database.")
		return nil
	}

	if _, err := dbConn.Model((*db.Schedule)(nil)).Where("TRUE").Delete(); err != nil {
		return fmt.Errorf("failed to delete schedules from database: %w", err)
	}

	if _, err := dbConn.Model(&schedules).Insert(); err != nil {
		return fmt.Errorf("failed to save schedules to database: %w", err)
	}

	if _, err := dbConn.Model(&db.Metadata{LastUpdate: lastUpdate}).Insert(); err != nil {
		return fmt.Errorf("failed to save metadata to database: %w", err)
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

	if val, exists := dayMap[day]; exists {
		return val
	}

	return 0
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

		re := regexp.MustCompile(`\d{2}\.\d{2}`)
		match := re.FindString(e.Text)

		startDateStr := match + fmt.Sprintf(".%d", time.Now().Year())
		startDate, err := time.Parse("02.01.2006", startDateStr)
		if err != nil {
			fmt.Printf("Error parsing date for week %s: %v\n", weekID, err)
			return
		}

		weekStartDates[weekID] = startDate
	})

	c.AllowURLRevisit = true
	err := visitWithRetry(c, link, 5, 2*time.Second)
	if err != nil {
		fmt.Println(err)
	}

	wg.Wait()

	return weekStartDates
}

func visitWithRetry(c *colly.Collector, link string, maxRetries int, delay time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		if err := c.Visit(link); err != nil {
			fmt.Printf("Failed to visit link %s: %v. Retrying in %v...\n", link, err, delay)
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to visit link %s after %d attempts", link, maxRetries)
}

func Start(dbConn *pg.DB) {
	if err := scrapeAndUpdate(dbConn); err != nil {
		fmt.Printf("Error during initial scraping and updating: %v\n", err)
	}

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := scrapeAndUpdate(dbConn); err != nil {
			fmt.Printf("Error during scraping and updating: %v\n", err)
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
		if err := processHTML(e, &latestUpdate, &groups, &mu); err != nil {
			fmt.Printf("Error processing HTML: %v\n", err)
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
		fmt.Printf("Visiting link: %s\n", link)
		if err := c.Visit(link); err != nil {
			fmt.Printf("Failed to visit link %s: %v\n", link, err)
		}
	}

	wg.Wait()

	return updateDatabaseIfNeeded(dbConn, latestUpdate, groups)
}

func processHTML(e *colly.HTMLElement, latestUpdate *time.Time, groups *[]string, mu *sync.Mutex) error {
	mainPageContent, err := e.DOM.Html()
	if err != nil {
		return fmt.Errorf("failed to get HTML content: %w", err)
	}

	lastUpdateDateFromWeb, err := fetchLastUpdateDateFromWeb(mainPageContent)
	if err != nil {
		return fmt.Errorf("failed to fetch last update date from web: %w", err)
	}

	mu.Lock()
	if lastUpdateDateFromWeb.After(*latestUpdate) {
		*latestUpdate = lastUpdateDateFromWeb
	}
	mu.Unlock()

	if e.Request.URL.String() == "https://www.polessu.by/ruz/?q=&f=1" {
		fetchedGroups, err := fetchGroups(mainPageContent)
		if err != nil {
			return fmt.Errorf("failed to fetch groups: %w", err)
		}
		*groups = fetchedGroups
	}

	return nil
}

func updateDatabaseIfNeeded(dbConn *pg.DB, latestUpdate time.Time, groups []string) error {
	lastUpdateDateFromDB, err := fetchLastUpdateDateFromDB(dbConn)
	if err != nil {
		return fmt.Errorf("failed to fetch last update date from database: %w", err)
	}

	fmt.Println("Date from DB: ", lastUpdateDateFromDB)
	fmt.Println("Latest date from web: ", latestUpdate)

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
			go func(g string) {
				defer wg.Done()
				parseScheduleForGroup(g, schedChan)
			}(group)
		}

		wg.Wait()
		close(schedChan)
		<-done

		if err := saveSchedulesToDB(dbConn, schedules, latestUpdate); err != nil {
			return fmt.Errorf("failed to save schedules to database: %w", err)
		}

		fmt.Println("Schedules saved to database.")
	}

	return nil
}
