package telegram_bot

import (
	"log"
	"time"

	"github.com/go-pg/pg/v10"
	"gopkg.in/telebot.v3"
)

func mainMenuButtons() *telebot.ReplyMarkup {
	mainMenu := &telebot.ReplyMarkup{}
	btnSchedule := mainMenu.Data("📆 Расписание", "schedule")
	btnSettings := mainMenu.Data("⚙️ Настройки", "settings")
	btnInformation := mainMenu.Data("ℹ️ Информация", "information")

	mainMenu.Inline(
		mainMenu.Row(btnSchedule),
		mainMenu.Row(btnSettings),
		mainMenu.Row(btnInformation),
	)
	return mainMenu
}

func scheduleMenuButtons() *telebot.ReplyMarkup {
	scheduleMenu := &telebot.ReplyMarkup{}
	btnScheduleNow := scheduleMenu.Data("📆 Сейчас", "now")
	btnBack := scheduleMenu.Data("⬅️ Назад", "back")

	scheduleMenu.Inline(
		scheduleMenu.Row(btnScheduleNow),
		scheduleMenu.Row(btnBack),
	)
	return scheduleMenu
}

func backMenuButtons() *telebot.ReplyMarkup {
	backMenu := &telebot.ReplyMarkup{}
	btnBack := backMenu.Data("⬅️ Назад", "back")

	backMenu.Inline(
		backMenu.Row(btnBack),
	)
	return backMenu
}

func termsOfServiceButtons() *telebot.ReplyMarkup {
	termsMenu := &telebot.ReplyMarkup{}
	btnAccept := termsMenu.Data("Принять", "accept_terms")
	btnDecline := termsMenu.Data("Отказаться", "decline_terms")

	termsMenu.Inline(
		termsMenu.Row(btnAccept, btnDecline),
	)
	return termsMenu
}

func handleCommands(bot *telebot.Bot) {
	mainMenu := mainMenuButtons()
	scheduleMenu := scheduleMenuButtons()
	settingsMenu := backMenuButtons()
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

	bot.Handle(&telebot.Btn{Unique: "information"}, func(c telebot.Context) error {
		log.Println("Information command")
		return c.Edit("Информация:", informationMenu)
	})
}

func StartBot(token string, dbConn *pg.DB) {
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

	handleCommands(bot)

	bot.Start()
}
