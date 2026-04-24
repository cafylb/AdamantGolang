package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	config "adamant/app/bot/config"
	core "adamant/app/bot/core"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	admin "adamant/app/bot/handlers/admin"
	user "adamant/app/bot/handlers/user"
	utils "adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	i18n.MustInit()

	bot, err := api.NewBot(config.Cfg.Token, api.WithDefaultDebugLogger())
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewPostgres(ctx, config.Cfg.DB)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := core.StartBackground(ctx, bot); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := core.StopBackground(); err != nil {
			log.Printf("background shutdown error: %v", err)
		}
	}()

	updates, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
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
