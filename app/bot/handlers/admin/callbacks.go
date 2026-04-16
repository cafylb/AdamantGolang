package admin

import (
	botpkg "adamant/app/bot"
	i18n "adamant/app/bot/core/i18n"
	utils "adamant/app/bot/utils"
	api "github.com/mymmrac/telego"
)

func HandleCallback(bot *api.Bot, callback *api.CallbackQuery) {
	utils.CallbackAnswer(bot, callback)
	lang := i18n.ForUser(callback.From.ID).Language()

	switch callback.Data {
	case "test":
		utils.Answer(bot, callback.From.ID, "Тест прошел успешно !", botpkg.MainPanel(lang))
	}
}
