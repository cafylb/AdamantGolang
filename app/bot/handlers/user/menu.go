package user

import (
	botpkg "adamant/app/bot"
	i18n "adamant/app/bot/core/i18n"
	utils "adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
)

func HandleCommand(bot *api.Bot, msg *api.Message) {
	tr := i18n.ForUser(msg.From.ID)

	switch utils.Commands(msg) {
	case "start": startHand(bot, msg, tr)
	case "language": languageHand(bot, msg, tr)
	case "id": Id(bot, msg, tr)
	}
}

func startHand(bot *api.Bot, msg *api.Message, tr i18n.Localizer) {
	utils.Answer(bot, msg.Chat.ID, tr.Get("menu.start").String(), botpkg.MainPanel(tr.Language()))
	utils.DeleteMessage(bot, msg)
}

func languageHand(bot *api.Bot, msg *api.Message, tr i18n.Localizer) {
	utils.Reply(bot, msg.Message(), tr.Get("menu.language").String(), botpkg.Language(tr.Language()))
}

func Id(bot *api.Bot, msg *api.Message, tr i18n.Localizer) {
	utils.Reply(bot, msg, tr.Get("menu.id").Format("tg_id", msg.From.ID), botpkg.Copy(tr.Language(), string(msg.From.ID)))
}