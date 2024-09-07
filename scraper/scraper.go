package scraper

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/go-pg/pg/v10"
	"github.com/gocolly/colly"
)

// Schedule структура для хранения расписания
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

// Глобальный слайс для хранения всех расписаний перед сохранением в базу
var allSchedules []Schedule
var mu sync.Mutex

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

func parseScheduleForGroup(group string) {
	fmt.Printf("Parsing schedule for group: %s\n", group)

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=1",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	for _, link := range links {
		link = link + "&q=" + group

		c := colly.NewCollector()

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
				match := re.FindStringSubmatch(weekClass)

				if len(match) < 1 {
					return
				}

				weekNumber := match[0]
				startDate, ok := weekStartDates[weekNumber]
				if !ok {
					fmt.Printf("No start date for week: %s\n", weekNumber)
					return
				}

				timeRange := el.ChildText("td:nth-child(1)")
				subjectInfo := el.ChildText("td:nth-child(2)")
				room := el.ChildText("td:nth-child(3)")
				teacher := el.ChildText("td:nth-child(4)")
				subgroup := el.ChildText("td:nth-child(5) span")

				dayOfWeek := calculateDayOfWeek(currentDay)
				classDate := startDate.AddDate(0, 0, dayOfWeek-1)

				schedule := Schedule{
					GroupName:  group,
					LessonDate: classDate.Format("02-01-2006"),
					DayOfWeek:  currentDay,
					LessonTime: timeRange,
					LessonName: subjectInfo,
					Location:   room,
					Teacher:    teacher,
					Subgroup:   subgroup,
				}

				mu.Lock()
				allSchedules = append(allSchedules, schedule)
				mu.Unlock()
			})
		})

		c.Visit(link)
	}
}

func saveSchedulesToDB(dbConn *pg.DB) error {
	_, err := dbConn.Model(&allSchedules).Insert()
	if err != nil {
		return fmt.Errorf("failed to save schedules to database: %w", err)
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
			log.Fatalf("Error parsing date for week %s: %v\n", weekID, err)
			return
		}

		weekStartDates[weekID] = startDate
	})

	c.Visit(link)

	wg.Wait()

	return weekStartDates
}

func Start(dbConn *pg.DB) {
	c := colly.NewCollector()

	var groups []string
	var latestUpdate time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup

	c.OnHTML("html", func(e *colly.HTMLElement) {
		defer wg.Done()

		mainPageContent, err := e.DOM.Html()
		if err != nil {
			log.Fatalf("Error getting main page content: %v", err)
		}

		lastUpdateDateFromWeb, err := fetchLastUpdateDateFromWeb(mainPageContent)
		if err != nil {
			log.Printf("Error fetching last update date from web: %v", err)
			return
		}

		mu.Lock()
		if lastUpdateDateFromWeb.After(latestUpdate) {
			latestUpdate = lastUpdateDateFromWeb
		}
		mu.Unlock()

		if e.Request.URL.String() == "https://www.polessu.by/ruz/?q=&f=1" {
			groups, err = fetchGroups(mainPageContent)
			if err != nil {
				log.Printf("Error fetching groups: %v", err)
				return
			}
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Request failed: %v", err)
	})

	links := []string{
		"https://www.polessu.by/ruz/?q=&f=1",
		"https://www.polessu.by/ruz/?q=&f=2",
		"https://www.polessu.by/ruz/term2/?q=&f=1",
		"https://www.polessu.by/ruz/term2/?q=&f=2",
	}

	for _, link := range links {
		wg.Add(1)
		log.Printf("Visiting link: %s", link)
		c.Visit(link)
	}

	wg.Wait()

	lastUpdateDateFromDB, err := fetchLastUpdateDateFromDB(dbConn)
	if err != nil {
		log.Fatalf("Error fetching last update date from database: %v", err)
	}

	log.Println("Date from DB: ", lastUpdateDateFromDB)
	log.Println("Latest date from web: ", latestUpdate)

	if latestUpdate.After(lastUpdateDateFromDB) {
		var wg2 sync.WaitGroup
		for _, group := range groups {
			wg2.Add(1)
			go func(g string) {
				defer wg2.Done()
				parseScheduleForGroup(g)
			}(group)
		}
		wg2.Wait()

		if err := saveSchedulesToDB(dbConn); err != nil {
			log.Fatalf("Error saving schedules to database: %v", err)
		}

		log.Println("Schedules saved to database.")
	}
}
