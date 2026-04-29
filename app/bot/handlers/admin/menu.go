package admin

import (
	botpkg "adamant/app/bot"
	i18n "adamant/app/bot/core/i18n"
	requests "adamant/app/bot/database/repository"
	utils "adamant/app/bot/utils"
	"context"
	"strconv"
	"strings"

	api "github.com/mymmrac/telego"
)

func HandleCommand(bot *api.Bot, msg *api.Message) {
	tr := i18n.ForUser(msg.From.ID)

	switch utils.Commands(msg) {
	case "admin": utils.Answer(bot, msg.Chat.ID, tr.Get("menu.admin.self").String(), botpkg.AdminPanel(tr.Language()))
	case "adamant": topUpAdamantBalance(bot, msg)	 
	}
	
}

func topUpAdamantBalance(bot *api.Bot, msg *api.Message) {
	data := strings.Fields(msg.Text)
	if len(data) != 3 {
		utils.Answer(bot, msg.Chat.ID, "Ошибка")
		return
	}

	userID, err := strconv.Atoi(data[1])
	if err != nil {
		utils.Answer(bot, msg.Chat.ID, "Ошибка")
		return
	}
	amount, err := strconv.Atoi(data[2])
	if err != nil {
		utils.Answer(bot, msg.Chat.ID, "Ошибка")
		return
	}

	requests.ChangeAdamantBalance(context.Background(), int64(userID), int64(amount))
	utils.Answer(bot, msg.Chat.ID, "Успешно")
}