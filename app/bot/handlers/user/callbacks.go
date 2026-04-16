package user

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	botpkg "adamant/app/bot"
	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	fsm "adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	repository "adamant/app/bot/database/repository"
	utils "adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
)

func HandleCallback(bot *api.Bot, callback *api.CallbackQuery) {
	tr := i18n.ForUser(callback.From.ID)
	data := callback.Data
	if strings.HasSuffix(data, "_P") {
		dataP := strings.Split(data, "_")
		system := dataP[2]
		amount, err := strconv.Atoi(dataP[0])
		if err != nil {
			fmt.Println(err)
		}
		username := dataP[1]
		switch system {
		case "ton":
			tonPurchase(bot, callback, tr, amount, username)
		case "usd":
			usdPurchase(bot, callback, tr, amount, username)
		case "uzs":
			uzsPurchase(bot, callback, tr, amount, username)
		case "adamant":
			adamantPurchase(bot, callback, tr, amount, username)
		case "cryptomus":
			cryptomusPurchase(bot, callback, tr, amount, username)
		}
	} else if strings.HasPrefix(data, "gifts_purchase") && data != "gifts_purchase" {
		page, err := strconv.Atoi(strings.TrimPrefix(data, "gifts_purchase_"))
		if err != nil {
			log.Println(err)
		}
		giftsPruchase(bot, callback, tr, page)
		return
	} else if strings.HasSuffix(data, "_back") {
		dataP := strings.Split(data, "_")
		amount, err := strconv.Atoi(dataP[0])
		if err != nil {
			log.Println(err)
		}
		username := dataP[1]
		payment(bot, callback, tr, amount, username)
	} else if strings.HasPrefix(data, "gift_purchase") {
		giftID, err := strconv.Atoi(strings.TrimPrefix(data, "gift_purchase_"))
		if err != nil {
			log.Println(err)
		}

		giftPurchase(bot, callback, tr, giftID)
	}

	switch data {
	case "purchase": purchase(bot, callback, tr)
	case "choose_language": chooseLanguage(bot, callback, tr)
	case "set_language_ru", "set_language_en", "set_language_uz": changeLanguage(bot, callback, tr, callback.Data)
	case "premium_purchase": premiumPurchase(bot, callback, tr)
	case "stars_purchase": starsPurchase(bot, callback, tr)
	case "gifts_purchase": giftsPruchase(bot, callback, tr)
	case "buy_stars_friend": buyStarsFriend(bot, callback, tr)
	case "buy_gifts_friend":
	case "cancel": cancelPurchase(bot, callback, tr)
	case "nothing": nothing(bot, callback, tr)
	}
}

func payment(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Payment("source").Format("adamant_balance", currentUserBalance(callback.From.ID)), botpkg.PaymentSource(tr.Language(), amount, username))
}

func purchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.self").String(), botpkg.BuyList(tr.Language()))
}

func premiumPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback, tr.Get("error.not_yet").String())
}

func giftPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, giftID int) {
	utils.CallbackAnswer(bot, callback)
	
}

func giftsPruchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, page ...int) {
	utils.CallbackAnswer(bot, callback)

	if len(page) == 0 {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.gifts.self").Format("username", callback.From.Username, "adamant_balance", currentUserBalance(callback.From.ID)), botpkg.GiftList(tr.Language(), page...))
	} else {
		utils.EditKeyboard(bot, callback.Message.Message(), *botpkg.GiftList(tr.Language(), page...))
	}
}

func chooseLanguage(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.language").String(), botpkg.Language(tr.Language()))
}

func changeLanguage(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	utils.CallbackAnswer(bot, callback)
	lang := strings.TrimPrefix(callback.Data, "set_language_")
	lang = i18n.SetUserLanguage(callback.From.ID, lang)
	tr = i18n.For(lang)
	utils.Edit(bot, callback.Message.Message(), tr.Main("start").String(), botpkg.MainPanel(tr.Language()))
}

func starsPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, user ...string) {
	utils.CallbackAnswer(bot, callback)
	username := callback.From.Username
	if len(user) > 0 && user[0] != "" {
		username = user[0]
	}

	if username != "" {
		fsm.UserFSM.SetState(callback.From.ID, fsm.StateAmount, callback.Message.Message().MessageID)
		fsm.UserFSM.SetUsername(callback.From.ID, username)
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.self").Format("MIN_STARS", config.Cfg.MIN_STARS, "MAX_STARS", config.Cfg.MAX_STARS, "username", username), botpkg.AnotherStar(tr.Language()))
	} else {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.username_self_error").String(), botpkg.AnotherStar(tr.Language()))
	}
}

func usdPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.slow.usd").String(), botpkg.UzdUsd(tr.Language(), amount, username))
}

func uzsPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.slow.uzs").String(), botpkg.UzdUsd(tr.Language(), amount, username))
}

func tonPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback)
	var amount_ton float32 = float32(amount)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.ton").Format("amount", amount, "username", username, "amount_ton", 12, "BANK", config.Cfg.Bank, "order_id", callback.From.ID), botpkg.PayTon(tr.Language(), utils.GenerateTONlink(config.Cfg.Bank, amount_ton, strconv.FormatInt(callback.From.ID, 10))))
}

func adamantPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback, tr.Get("error.balance").String())
}

func cryptomusPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, amount int, username string) {
	utils.CallbackAnswer(bot, callback)
}

func buyGiftsFriend(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
}

func buyStarsFriend(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.SetState(callback.From.ID, fsm.StateUsername, callback.Message.Message().MessageID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.username").String(), botpkg.Cancel(tr.Language()))
}

func nothing(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.start").String(), botpkg.MainPanel(tr.Language()))
}

func cancelPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	purchase(bot, callback, tr)
}

func currentUserBalance(userID int64) int64 {
	balance, err := repository.GetUserBalance(context.Background(), database.Pool, userID)
	if err != nil {
		fmt.Println(err)
	}

	return balance
}