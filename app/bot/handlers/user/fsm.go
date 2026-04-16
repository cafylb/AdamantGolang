package user

import (
	botpkg "adamant/app/bot"
	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	repository "adamant/app/bot/database/repository"
	"adamant/app/bot/utils"
	"context"
	"strconv"

	api "github.com/mymmrac/telego"
)

func HandleFSM(bot *api.Bot, message *api.Message, data string) {
	switch data {
	case string(fsm.StateUsername):
		waitUsername(bot, message)
	case string(fsm.StateAmount):
		waitAmount(bot, message)
	}
}

func waitUsername(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)
	username := message.Text
	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return
	}

	utils.DeleteMessage(bot, message)
	utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("menu.buy_list.stars.self").Format("MIN_STARS", config.Cfg.MIN_STARS, "MAX_STARS", config.Cfg.MAX_STARS, "username", username), botpkg.AnotherStar(tr.Language()))
	session.State = fsm.StateAmount
	fsm.UserFSM.Set(message.From.ID, session)
	fsm.UserFSM.SetUsername(message.From.ID, username)
}

func waitAmount(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)

	amount, err := strconv.Atoi(message.Text)
	if err != nil || !(config.Cfg.MIN_STARS <= int32(amount) && int32(amount) <= config.Cfg.MAX_STARS) {
		return
	}

	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return
	}

	username, ok := fsm.UserFSM.GetUsername(message.From.ID)
	if !ok || username == "" {
		return
	}

	utils.DeleteMessage(bot, message)
	utils.EditById(
		bot,
		int64(session.MessageID),
		message.Chat.ID,
		tr.Payment("source").Format("adamant_balance", fsmUserBalance(message.From.ID)),
		botpkg.PaymentSource(tr.Language(), amount, username),
	)
	fsm.UserFSM.Clear(message.From.ID)
}

func fsmUserBalance(userID int64) string {
	if database.Pool == nil {
		return "0"
	}

	balance, err := repository.GetUserBalance(context.Background(), database.Pool, userID)
	if err != nil {
		return "0"
	}

	return string(balance)
}
