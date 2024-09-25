package telegram_bot

import (
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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

	previousDay := currentDay.AddDate(0, 0, -1)
	nextDay := currentDay.AddDate(0, 0, 1)

	return createMenu(5,
		createButton("<<", "now", previousMonday.Format("02.01.2006")),
		createButton("<", "now", previousDay.Format("02.01.2006")),
		createButton("●", "now", ""),
		createButton(">", "now", nextDay.Format("02.01.2006")),
		createButton(">>", "now", nextMonday.Format("02.01.2006")),
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
		return c.Send("*Отказ от ответственности*\n\nИнформация, предоставляемая ботом, носит справочный характер. Мы не несем ответственности за точность, полноту или актуальность данных. Использование информации осуществляется на ваш собственный риск.\n\nНажмите кнопку ниже, чтобы принять правила:", termsOfServiceButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "accept_terms"}, func(c telebot.Context) error {
		return c.Edit("Благодарим вас за принятие условий предоставления услуг. Добро пожаловать в бот!", mainMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "decline_terms"}, func(c telebot.Context) error {
		return c.Edit("Чтобы использовать этого бота, вам необходимо принять условия предоставления услуг")
	})

	bot.Handle(&telebot.Btn{Unique: "schedule"}, func(c telebot.Context) error {
		return c.Edit("Меню расписания:", scheduleMenuButtons())
	})

	bot.Handle(&telebot.Btn{Unique: "now"}, func(c telebot.Context) error {
		return handleNowButton(c, dbConn)
	})

	bot.Handle(&telebot.Btn{Unique: "back"}, func(c telebot.Context) error {
		return c.Edit("Главное меню:", mainMenuButtons())
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
	if err != nil {
		return c.Edit("Ошибка получения информации о пользователе.")
	}
	if user.GroupName == "" {
		return c.Edit("Вы не выбрали группу для просмотра расписания.")
	}

	todayTime, todayStr, err := parseDate(c.Data())
	if err != nil {
		todayTime = time.Now()
		todayStr = todayTime.Format("02.01.2006")
	}

	schedules, err := getSchedule(dbConn, user.GroupName, todayTime)
	if err != nil {
		return c.Edit(fmt.Sprintf("Ошибка получения расписания: %v", err))
	}
	if len(schedules) == 0 {
		return c.Edit(fmt.Sprintf("Расписание не найдено на дату %s", todayStr[:5]), scheduleNowMenuButtons(todayTime))
	}

	text := formatSchedule(schedules, todayTime)

	if user.IsBanned {
		text = shuffleString(text)
	}

	return c.Edit(text, scheduleNowMenuButtons(todayTime))
}

func shuffleString(s string) string {
	runes := []rune(s)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	var letters []rune
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters = append(letters, r)
		}
	}

	for i := range letters {
		j := r.Intn(len(letters))
		letters[i], letters[j] = letters[j], letters[i]
	}

	var result strings.Builder
	lettersIndex := 0
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(letters[lettersIndex])
			lettersIndex++
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

func parseDate(dateStr string) (time.Time, string, error) {
	if dateStr == "" {
		t := time.Now()
		return t, t.Format("02.01.2006"), nil
	}

	t, err := time.Parse("02.01.2006", dateStr)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid date format: %w", err)
	}
	return t, dateStr, nil
}

func formatSchedule(schedules []db.Schedule, todayTime time.Time) string {
	todayStr := todayTime.Format("02.01")
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
	var yearButtons [][]telebot.Btn
	for _, year := range years {
		btn := createButton(year, "select_year", year)
		yearButtons = append(yearButtons, btn)
	}
	return createMenu(1, yearButtons...)
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
	var specButtons [][]telebot.Btn
	for _, spec := range specs {
		btn := createButton(spec, "select_spec", selectedYear+"_"+spec)
		specButtons = append(specButtons, btn)
	}
	return createMenu(3, specButtons...)
}

func handleSelectSpec(c telebot.Context, dbConn *pg.DB) error {
	data := strings.Split(c.Data(), "_")
	if len(data) < 2 {
		return c.Edit("Ошибка: некорректные данные для специальности.")
	}
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
	var groupButtons [][]telebot.Btn
	for _, group := range groups {
		btn := createButton(group, "select_group", group)
		groupButtons = append(groupButtons, btn)
	}
	return createMenu(1, groupButtons...)
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

func getSchedule(dbConn *pg.DB, groupName string, day time.Time) ([]db.Schedule, error) {
	var schedules []db.Schedule

	dayStr := day.Format("02.01")

	err := dbConn.Model(&schedules).
		Where("group_name = ?", groupName).
		Where("lesson_date = ?", dayStr).
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
		fmt.Printf("Failed to create bot: %v\n", err)
		return
	}

	handleCommands(bot, dbConn)
	bot.Start()
}
