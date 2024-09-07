package telegram_bot

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Ah3ron/schedule-bot/db"
	"github.com/go-pg/pg/v10"
	"gopkg.in/telebot.v3"
)

func createButton(text, unique string) []telebot.Btn {
	return []telebot.Btn{
		{Text: text, Unique: unique},
	}
}

func createMenu(buttonsInRow int, buttonGroups ...[]telebot.Btn) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	var allButtons []telebot.Btn

	for _, group := range buttonGroups {
		allButtons = append(allButtons, group...)
	}

	var rows []telebot.Row
	for i := 0; i < len(allButtons); i += buttonsInRow {
		end := i + buttonsInRow
		if end > len(allButtons) {
			end = len(allButtons)
		}
		row := allButtons[i:end]
		rows = append(rows, menu.Row(row...))
	}

	menu.Inline(rows...)
	return menu
}

func mainMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("📆 Расписание", "schedule"),
		createButton("⚙️ Настройки", "settings"),
		createButton("ℹ️ Информация", "information"),
	)
}

func scheduleMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("📆 Сейчас", "now"),
		createButton("⬅️ Назад", "back"),
	)
}

func settingsMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("🔄 Выбрать группу", "choose_group"),
		createButton("⬅️ Назад", "back"),
	)
}

func backMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("⬅️ Назад", "back"),
	)
}

func termsOfServiceButtons() *telebot.ReplyMarkup {
	return createMenu(
		2,
		createButton("Принять", "accept_terms"),
		createButton("Отказаться", "decline_terms"),
	)
}

func getUniqueGroups(dbConn *pg.DB) ([]string, error) {
	var groups []string
	err := dbConn.Model((*db.Schedule)(nil)).ColumnExpr("DISTINCT group_name").Select(&groups)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch unique groups: %w", err)
	}
	return groups, nil
}

func parseGroupName(group string) (year string, spec string, name string) {
	re := regexp.MustCompile(`^(\d{2})([а-яА-Я]+)-(\d+)[а-я]*$`)
	matches := re.FindStringSubmatch(group)
	if len(matches) != 4 {
		return "", "", group
	}
	return matches[1], matches[2], matches[1] + matches[2] + "-" + matches[3]
}

func getAdmissionYears(groups []string) []string {
	yearSet := make(map[string]struct{})
	for _, group := range groups {
		year, _, _ := parseGroupName(group)
		if year != "" {
			yearSet[year] = struct{}{}
		}
	}

	years := make([]string, 0, len(yearSet))
	for year := range yearSet {
		years = append(years, year)
	}
	sort.Strings(years)

	return years
}

// Extract specializations from group names based on the selected year
func getSpecializations(groups []string, year string) []string {
	specSet := make(map[string]struct{})
	for _, group := range groups {
		groupYear, spec, _ := parseGroupName(group)
		if groupYear == year {
			specSet[spec] = struct{}{}
		}
	}

	specs := make([]string, 0, len(specSet))
	for spec := range specSet {
		specs = append(specs, spec)
	}
	sort.Strings(specs)

	return specs
}

func getGroups(groups []string, year, spec string) []string {
	groupSet := make(map[string]struct{})
	for _, group := range groups {
		groupYear, groupSpec, fullGroup := parseGroupName(group)
		if groupYear == year && groupSpec == spec {
			groupSet[fullGroup] = struct{}{}
		}
	}

	finalGroups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		finalGroups = append(finalGroups, group)
	}

	sort.Strings(finalGroups)
	return finalGroups
}

func handleCommands(bot *telebot.Bot, dbConn *pg.DB) {
	mainMenu := mainMenuButtons()
	scheduleMenu := scheduleMenuButtons()
	settingsMenu := settingsMenuButtons()
	informationMenu := backMenuButtons()
	termsMenu := termsOfServiceButtons()

	bot.Handle("/start", func(c telebot.Context) error {
		log.Println("Start command")
		return c.Send("Пожалуйста, примите условия предоставления услуг, чтобы продолжить.", termsMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "accept_terms"}, func(c telebot.Context) error {
		log.Println("Terms accepted")
		return c.Edit("Благодарим вас за принятие условий предоставления услуг. Добро пожаловать в бот!", mainMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "decline_terms"}, func(c telebot.Context) error {
		log.Println("Terms declined")
		return c.Edit("Чтобы использовать этого бота, вам необходимо принять условия предоставления услуг.")
	})

	bot.Handle(&telebot.Btn{Unique: "schedule"}, func(c telebot.Context) error {
		log.Println("Schedule command")
		return c.Edit("Вот ваше меню расписания:", scheduleMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "now"}, func(c telebot.Context) error {
		log.Println("Schedule now command")
		return c.Edit("Вот ваше расписание на данный момент:", scheduleMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "back"}, func(c telebot.Context) error {
		log.Println("Back command")
		return c.Edit("Вернуться в главное меню:", mainMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "settings"}, func(c telebot.Context) error {
		log.Println("Settings command")
		return c.Edit("Настройки:", settingsMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "choose_group"}, func(c telebot.Context) error {
		log.Println("Choose group command")

		uniqueGroups, err := getUniqueGroups(dbConn)
		if err != nil {
			return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
		}

		years := getAdmissionYears(uniqueGroups)
		yearButtons := &telebot.ReplyMarkup{}

		var yearRows []telebot.Row
		for _, year := range years {
			btn := yearButtons.Data(year, "select_year", year)
			yearRows = append(yearRows, yearButtons.Row(btn))
		}

		yearButtons.Inline(yearRows...)

		return c.Edit("Выберите год поступления:", yearButtons)
	})

	bot.Handle(&telebot.Btn{Unique: "select_year"}, func(c telebot.Context) error {
		selectedYear := c.Data()
		log.Printf("Selected year: %s", selectedYear)

		uniqueGroups, err := getUniqueGroups(dbConn)
		if err != nil {
			return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
		}

		specs := getSpecializations(uniqueGroups, selectedYear)
		specButtons := &telebot.ReplyMarkup{}

		var specRows []telebot.Row
		for _, spec := range specs {
			btn := specButtons.Data(spec, "select_spec", selectedYear+"_"+spec)
			specRows = append(specRows, specButtons.Row(btn))
		}

		specButtons.Inline(specRows...)

		return c.Edit("Выберите поток:", specButtons)
	})

	bot.Handle(&telebot.Btn{Unique: "select_spec"}, func(c telebot.Context) error {
		data := strings.Split(c.Data(), "_")
		selectedYear := data[0]
		selectedSpec := data[1]
		log.Printf("Selected spec: %s for year %s", selectedSpec, selectedYear)

		uniqueGroups, err := getUniqueGroups(dbConn)
		if err != nil {
			return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
		}

		groups := getGroups(uniqueGroups, selectedYear, selectedSpec)
		groupButtons := &telebot.ReplyMarkup{}

		var groupRows []telebot.Row
		for _, group := range groups {
			btn := groupButtons.Data(group, "select_group", group)
			groupRows = append(groupRows, groupButtons.Row(btn))
		}

		groupButtons.Inline(groupRows...)

		return c.Edit("Выберите группу:", groupButtons)
	})

	bot.Handle(&telebot.Btn{Unique: "select_group"}, func(c telebot.Context) error {
		selectedGroup := c.Data()

		user := &db.Users{
			TelegramID: c.Sender().ID,
			GroupName:  selectedGroup,
		}

		_, err := dbConn.Model(user).
			OnConflict("(telegram_id) DO UPDATE").
			Set("group_name = EXCLUDED.group_name").
			Insert()
		if err != nil {
			return c.Edit(fmt.Sprintf("Ошибка сохранения группы: %v", err))
		}

		return c.Edit(fmt.Sprintf("Ваша группа была успешно выбрана: %s", selectedGroup), mainMenu)
	})

	bot.Handle(&telebot.Btn{Unique: "information"}, func(c telebot.Context) error {
		log.Println("Information command")
		return c.Edit("Информация:", informationMenu)
	})
}

func Start(token string, dbConn *pg.DB) {
	opts := telebot.Settings{
		Token: token,
		Poller: &telebot.LongPoller{
			Timeout: 10 * time.Second,
			AllowedUpdates: []string{
				"message",
				"edited_message",
				"channel_post",
				"edited_channel_post",
				"message_reaction",
				"message_reaction_count",
				"inline_query",
				"chosen_inline_result",
				"callback_query",
				"shipping_query",
				"pre_checkout_query",
				"poll",
				"poll_answer",
				"my_chat_member",
				"chat_member",
				"chat_join_request",
				"chat_boost",
				"removed_chat_boost",
			},
		},
	}

	bot, err := telebot.NewBot(opts)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	handleCommands(bot, dbConn)

	bot.Start()
}
