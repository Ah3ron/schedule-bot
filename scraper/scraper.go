package scraper

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-pg/pg/v10"
)

func fetchURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
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
	mainPageContent, err := fetchURL("https://www.polessu.by/ruz/")
	if err != nil {
		log.Fatalf("Error fetching main page: %v", err)
	}

	groups, err := fetchGroups(mainPageContent)
	if err != nil {
		log.Fatalf("Error fetching groups: %v", err)
	}

	for group := range groups {
		fmt.Println(groups[group])
	}
}
