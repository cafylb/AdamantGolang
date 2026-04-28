package user

import (
	botpkg "adamant/app/bot"
	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	repository "adamant/app/bot/database/repository"
	"adamant/app/bot/utils"
	services "adamant/app/bot/services"
	"context"
	"strconv"
	"strings"

	api "github.com/mymmrac/telego"
)

func HandleFSM(bot *api.Bot, message *api.Message, data string) {
	switch data {
	case string(fsm.StateUsernameStars):
		waitUsernameStars(bot, message)
	case string(fsm.StateUsernamePremium):
		waitUsernamePremium(bot, message)
	case string(fsm.StateUsernameGifts):
		waitUsernameGifts(bot, message)
	case string(fsm.StateAmount):
		waitAmount(bot, message)
	case string(fsm.StateMemo):
		waitGiftMemo(bot, message)
	}
}

func waitUsernameStars(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)
	username := message.Text
	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return
	}
	_, err := services.FragmentService.CheckUsername(context.Background(), username)
	if err != nil {
		utils.DeleteMessage(bot, message)
		utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("error.username").String(), botpkg.Cancel(tr.Language()))
		return
	}

	utils.DeleteMessage(bot, message)
	utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("menu.buy_list.stars.self").Format("MIN_STARS", config.Cfg.MIN_STARS, "MAX_STARS", config.Cfg.MAX_STARS, "username", username), botpkg.AnotherPurchase(tr.Language(), "stars"))
	session.State = fsm.StateAmount
	session.Username = username
	fsm.UserFSM.Set(message.From.ID, session)
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

	go utils.DeleteMessage(bot, message)
	utils.EditById(bot, int64(session.MessageID), message.Chat.ID, tr.Payment("source").Format("adamant_balance", fsmUserBalance(message.From.ID)), botpkg.PaymentSource(tr.Language(), utils.GenerateOrderData("stars", amount, username)))
	fsm.UserFSM.Clear(message.From.ID)
}

func waitUsernamePremium(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)
	username := strings.TrimSpace(message.Text)
	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return	
	}

	isPremium, user, err := services.FragmentService.CheckPremium(context.Background(), username, 3)
	if user == "" || err != nil {
		go utils.DeleteMessage(bot, message)
		utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("error.username").String(), botpkg.Cancel(tr.Language()))
		return
	} else if isPremium {
		go utils.DeleteMessage(bot, message)
		utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("menu.buy_list.premium.already_has").String(), botpkg.Cancel(tr.Language()))
		return
	}

	go utils.EditById(bot, int64(session.MessageID), message.Chat.ID, tr.Get("menu.buy_list.premium.self").Format("username", username),botpkg.Premium(tr.Language()))
	utils.DeleteMessage(bot, message)
	session.State = fsm.StateIdle
	session.Username = username
	fsm.UserFSM.Set(message.From.ID, session)
}

func waitUsernameGifts(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)
	username := strings.TrimSpace(message.Text)
	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return
	}

	_, err := services.FragmentService.CheckUsername(context.Background(), username)
	if err != nil {
		utils.DeleteMessage(bot, message)
		utils.EditById(bot, int64(session.MessageID), message.From.ID, tr.Get("error.username").String(), botpkg.Cancel(tr.Language()))
		return
	}

	go utils.DeleteMessage(bot, message)
	utils.EditById(bot, int64(session.MessageID), message.Chat.ID, tr.Get("menu.buy_list.gifts.self").Format("username", username, "adamant_balance", fsmUserBalance(message.From.ID)), botpkg.GiftList(tr.Language()))
	session.State = fsm.StateIdle
	session.Username = username
	fsm.UserFSM.Set(message.From.ID, session)
}

func waitGiftMemo(bot *api.Bot, message *api.Message) {
	tr := i18n.ForUser(message.From.ID)
	memo := strings.TrimSpace(message.Text)
	if memo == "" || len([]rune(memo)) > 255 {
		return
	}

	session, ok := fsm.UserFSM.Get(message.From.ID)
	if !ok {
		return
	}

	username := session.Username
	if username == "" {
		return
	}

	utils.DeleteMessage(bot, message)
	session.Memo = memo
	session.State = fsm.StateIdle
	fsm.UserFSM.Set(message.From.ID, session)

	utils.EditById(
		bot,
		int64(session.MessageID),
		message.Chat.ID,
		tr.Payment("source").Format("adamant_balance", fsmUserBalance(message.From.ID)),
		botpkg.PaymentSource(tr.Language(), utils.GenerateOrderData("gifts", session.GiftID, username)),
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

	return strconv.FormatInt(balance, 10)
}
