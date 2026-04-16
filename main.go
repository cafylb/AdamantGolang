package main

import (
	"context"
	"fmt"
	"log"

	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	admin "adamant/app/bot/handlers/admin"
	user "adamant/app/bot/handlers/user"
	utils "adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
)

func main() {
	i18n.MustInit()

	bot, err := api.NewBot(config.Cfg.Token, api.WithDefaultDebugLogger())
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewPostgres(context.Background(), config.Cfg.DB)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	updates, err := bot.UpdatesViaLongPolling(context.Background(), nil)

	fmt.Println("Запуск бота прошел успешно !")
	for update := range updates {
		if update.CallbackQuery != nil {
			go user.HandleCallback(bot, update.CallbackQuery)
			if utils.CheckAdmin(update.CallbackQuery.From.ID) {
				go admin.HandleCallback(bot, update.CallbackQuery)
				continue
			}
		}

		if update.Message == nil {
			continue
		}

		if utils.IsCommand(update.Message) {
			go user.HandleCommand(bot, update.Message)
			if utils.CheckAdmin(update.Message.From.ID) {
				go admin.HandleCommand(bot, update.Message)
				continue
			}
		}

		if cur := fsm.UserFSM.Has(update.Message.From.ID); cur != "" {
			user.HandleFSM(bot, update.Message, cur)
		}
	}
}
