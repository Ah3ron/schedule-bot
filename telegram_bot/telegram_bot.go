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

func createButton(text, unique, data string) []telebot.Btn {
	return []telebot.Btn{
		{Text: text, Unique: unique, Data: data},
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
		createButton("📆 Расписание", "schedule", ""),
		createButton("⚙️ Настройки", "settings", ""),
		createButton("ℹ️ Информация", "information", ""),
	)
}

func scheduleMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("📆 Сейчас", "now", ""),
		createButton("⬅️ Назад", "back", ""),
	)
}

func scheduleNowMenuButtons(currentDay time.Time) *telebot.ReplyMarkup {
	currentMonday := currentDay
	for currentMonday.Weekday() != time.Monday {
		currentMonday = currentMonday.AddDate(0, 0, -1)
	}

	previousMonday := currentMonday.AddDate(0, 0, -7)
	nextMonday := currentMonday.AddDate(0, 0, 7)

	return createMenu(5,
		createButton(" << ", "now", previousMonday.Format("02.01")),
		createButton(" < ", "now", currentDay.AddDate(0, 0, -1).Format("02.01")),
		createButton("Сегодня", "now", ""),
		createButton(" > ", "now", currentDay.AddDate(0, 0, 1).Format("02.01")),
		createButton(" >> ", "now", nextMonday.Format("02.01")),
		createButton("⬅️ Назад", "back", ""),
	)
}

func settingsMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("🔄 Выбрать группу", "choose_group", ""),
		createButton("⬅️ Назад", "back", ""),
	)
}

func backMenuButtons() *telebot.ReplyMarkup {
	return createMenu(1,
		createButton("⬅️ Назад", "back", ""),
	)
}

func termsOfServiceButtons() *telebot.ReplyMarkup {
	return createMenu(2,
		createButton("Принять", "accept_terms", ""),
		createButton("Отказаться", "decline_terms", ""),
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

func parseGroupName(group string) (year, spec, name string) {
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
		if year, _, _ := parseGroupName(group); year != "" {
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

func getSpecializations(groups []string, year string) []string {
	specSet := make(map[string]struct{})
	for _, group := range groups {
		if groupYear, spec, _ := parseGroupName(group); groupYear == year {
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
		if groupYear, groupSpec, fullGroup := parseGroupName(group); groupYear == year && groupSpec == spec {
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
	bot.Handle("/start", func(c telebot.Context) error {
		return c.Send("Пожалуйста, примите условия предоставления услуг, чтобы продолжить", termsOfServiceButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "accept_terms"}, func(c telebot.Context) error {
		return c.Edit("Благодарим вас за принятие условий предоставления услуг. Добро пожаловать в бот!", mainMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "decline_terms"}, func(c telebot.Context) error {
		return c.Edit("Чтобы использовать этого бота, вам необходимо принять условия предоставления услуг")
	})

	bot.Handle(&telebot.Btn{Unique: "schedule"}, func(c telebot.Context) error {
		return c.Edit("Вот ваше меню расписания:", scheduleMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "now"}, func(c telebot.Context) error {
		return handleNowButton(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "back"}, func(c telebot.Context) error {
		return c.Edit("Вернуться в главное меню:", mainMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "settings"}, func(c telebot.Context) error {
		return c.Edit("Настройки:", settingsMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "choose_group"}, func(c telebot.Context) error {
		return handleChooseGroup(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "select_year"}, func(c telebot.Context) error {
		return handleSelectYear(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "select_spec"}, func(c telebot.Context) error {
		return handleSelectSpec(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "select_group"}, func(c telebot.Context) error {
		return handleSelectGroup(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "information"}, func(c telebot.Context) error {
		return c.Edit("Информация:", backMenuButtons())
	})
}

func handleNowButton(c telebot.Context, dbConn *pg.DB) error {
	userID := c.Sender().ID
	user, err := getUserInfo(dbConn, userID)
	if err != nil || user.GroupName == "" {
		return c.Edit("Вы не выбрали группу для просмотра расписания.")
	}

	todayStr := c.Data()
	var todayTime time.Time
	if todayStr == "" {
		todayTime = time.Now()
		todayStr = todayTime.Format("02.01")
	} else {
		todayTime, _ = time.Parse("02.01", todayStr)
	}

	schedules, err := getSchedule(dbConn, user.GroupName, todayStr)
	if err != nil || len(schedules) == 0 {
		return c.Edit("Расписание не найдено", scheduleNowMenuButtons(todayTime))
	}

	text := formatSchedule(schedules, todayStr)
	return c.Edit(text, scheduleNowMenuButtons(todayTime))
}

func formatSchedule(schedules []db.Schedule, todayStr string) string {
	text := fmt.Sprintf("Ваше расписание (%s, %s)\n", schedules[0].DayOfWeek, todayStr)

	for _, schedule := range schedules {
		text += fmt.Sprintf("\n*Время:* _%s_\n*Пара:* _%s_", schedule.LessonTime, schedule.LessonName)
		if schedule.Location != "" {
			text += fmt.Sprintf("\n*Аудит.:* _%s_", schedule.Location)
		}
		if schedule.Teacher != "" {
			text += fmt.Sprintf("\n*Препод.:* _%s_", schedule.Teacher)
		}
		if schedule.Subgroup != "" {
			text += fmt.Sprintf("\n*Подгруппа:* _%s_", schedule.Subgroup)
		}
		text += "\n"
	}
	return text
}

func handleChooseGroup(c telebot.Context, dbConn *pg.DB) error {
	uniqueGroups, err := getUniqueGroups(dbConn)
	if err != nil {
		return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
	}

	years := getAdmissionYears(uniqueGroups)
	yearButtons := createYearButtons(years)
	return c.Edit("Выберите год поступления:", yearButtons)
}

func createYearButtons(years []string) *telebot.ReplyMarkup {
	yearButtons := &telebot.ReplyMarkup{}

	var yearRows []telebot.Row
	for _, year := range years {
		btn := yearButtons.Data(year, "select_year", year)
		yearRows = append(yearRows, yearButtons.Row(btn))
	}
	yearButtons.Inline(yearRows...)
	return yearButtons
}

func handleSelectYear(c telebot.Context, dbConn *pg.DB) error {
	selectedYear := c.Data()
	uniqueGroups, err := getUniqueGroups(dbConn)
	if err != nil {
		return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
	}

	specs := getSpecializations(uniqueGroups, selectedYear)
	specButtons := createSpecButtons(specs, selectedYear)
	return c.Edit("Выберите поток:", specButtons)
}

func createSpecButtons(specs []string, selectedYear string) *telebot.ReplyMarkup {
	specButtons := &telebot.ReplyMarkup{}

	var specRows []telebot.Row
	for _, spec := range specs {
		btn := specButtons.Data(spec, "select_spec", selectedYear+"_"+spec)
		specRows = append(specRows, specButtons.Row(btn))
	}
	specButtons.Inline(specRows...)
	return specButtons
}

func handleSelectSpec(c telebot.Context, dbConn *pg.DB) error {
	data := strings.Split(c.Data(), "_")
	selectedYear, selectedSpec := data[0], data[1]

	uniqueGroups, err := getUniqueGroups(dbConn)
	if err != nil {
		return c.Edit(fmt.Sprintf("Ошибка получения групп: %v", err))
	}

	groups := getGroups(uniqueGroups, selectedYear, selectedSpec)
	groupButtons := createGroupButtons(groups)
	return c.Edit("Выберите группу:", groupButtons)
}

func createGroupButtons(groups []string) *telebot.ReplyMarkup {
	groupButtons := &telebot.ReplyMarkup{}

	var groupRows []telebot.Row
	for _, group := range groups {
		btn := groupButtons.Data(group, "select_group", group)
		groupRows = append(groupRows, groupButtons.Row(btn))
	}
	groupButtons.Inline(groupRows...)
	return groupButtons
}

func handleSelectGroup(c telebot.Context, dbConn *pg.DB) error {
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

	return c.Edit(fmt.Sprintf("Ваша группа была успешно выбрана: %s", selectedGroup), mainMenuButtons())
}

func getUserInfo(dbConn *pg.DB, userID int64) (*db.Users, error) {
	var user db.Users
	err := dbConn.Model(&db.Users{}).Where("telegram_id = ?", userID).Select(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	return &user, nil
}

func getSchedule(dbConn *pg.DB, groupName string, day string) ([]db.Schedule, error) {
	var schedules []db.Schedule
	err := dbConn.Model(&schedules).
		Where("group_name = ?", groupName).
		Where("lesson_date = ?", day).
		Select()
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}
	return schedules, nil
}

func Start(token string, dbConn *pg.DB) {
	opts := telebot.Settings{
		Token:     token,
		ParseMode: "Markdown",
		Poller: &telebot.LongPoller{
			Timeout: 3 * time.Second,
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
