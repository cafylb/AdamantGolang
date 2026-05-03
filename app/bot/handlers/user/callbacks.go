package user

import (
	"context"
	"log"
	"strconv"
	"strings"

	botpkg "adamant/app/bot"
	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	fsm "adamant/app/bot/data/fsm"
	repository "adamant/app/bot/database/repository"
	utils "adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
)

func HandleCallback(bot *api.Bot, callback *api.CallbackQuery) {
	tr := i18n.ForUser(callback.From.ID)
	data := callback.Data

	if strings.HasPrefix(data, "pay_") {
		payData := strings.TrimPrefix(data, "pay_")
		dataP := strings.SplitN(payData, "_", 2)
		if len(dataP) != 2 {
			return
		}

		system := dataP[0]
		orderData := dataP[1]
		switch system {
		case "ton": tonPurchase(bot, callback, tr, orderData)
		case "usd": usdPurchase(bot, callback, tr, orderData)
		case "uzs": uzsPurchase(bot, callback, tr, orderData)
		case "adamant": adamantPurchase(bot, callback, tr, orderData)
		case "cryptomus": cryptomusPurchase(bot, callback, tr, orderData)
		}
		return
	} else if strings.HasPrefix(data, "gifts_purchase") && data != "gifts_purchase" {
		page, err := strconv.Atoi(strings.TrimPrefix(data, "gifts_purchase_"))
		if err != nil {
			log.Println(err)
		}
		giftsPruchase(bot, callback, tr, page)
		return
	} else if strings.HasPrefix(data, "payment_") {
		payment(bot, callback, tr, strings.TrimPrefix(data, "payment_"))
		return
	} else if strings.HasPrefix(data, "gift_purchase") {
		dataP := strings.Split(strings.TrimPrefix(data, "gift_purchase_"), "_")
		giftID, err := strconv.Atoi(dataP[0])
		if err != nil {
			log.Println(err)
		}
		anonimInt, err := strconv.Atoi(dataP[1])
		if err != nil {
			log.Println(err)
		}
		giftPurchase(bot, callback, tr, giftID, anonimInt != 0)
		return
	} else if strings.HasPrefix(data, "gift_comment_") {
		dataP := strings.Split(strings.TrimPrefix(data, "gift_comment_"), "_")
		if len(dataP) != 2 {
			return
		}

		giftID, err := strconv.Atoi(dataP[0])
		if err != nil {
			log.Println(err)
			return
		}

		anonymous := dataP[1] == "true"
		giftComment(bot, callback, tr, giftID, anonymous)
		return
	} else if strings.HasPrefix(data, "gift_without_comment_") {
		dataP := strings.Split(strings.TrimPrefix(data, "gift_without_comment_"), "_")
		if len(dataP) != 2 {
			return
		}

		giftID, err := strconv.Atoi(dataP[0])
		if err != nil {
			log.Println(err)
			return
		}

		anonymous := dataP[1] == "true"
		giftWithoutComment(bot, callback, tr, giftID, anonymous)
		return
	} else if strings.HasPrefix(data, "premium_purchase_") {
		premiumLength, err := strconv.Atoi(strings.TrimPrefix(data, "premium_purchase_"))
		if err != nil {
			log.Println(err)
			return
		}

		premiumDurationPurchase(bot, callback, tr, premiumLength)
		return
	}

	switch data {
	case "purchase":
		purchase(bot, callback, tr)
	case "choose_language":
		chooseLanguage(bot, callback, tr)
	case "set_language_ru", "set_language_en", "set_language_uz":
		changeLanguage(bot, callback, tr, callback.Data)
	case "premium_purchase":
		premiumPurchase(bot, callback, tr)
	case "stars_purchase":
		starsPurchase(bot, callback, tr)
	case "gifts_purchase":
		giftsPruchase(bot, callback, tr)
	case "buy_stars_friend":
		buyStarsFriend(bot, callback, tr)
	case "buy_gifts_friend":
		buyGiftsFriend(bot, callback, tr)
	case "buy_premium_friend": buyPremiumFriend(bot, callback, tr)
	case "cancel": cancelPurchase(bot, callback, tr)
	case "nothing": nothing(bot, callback, tr)
	}
}

func payment(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Payment("source").Format("adamant_balance", currentUserBalance(callback.From.ID)), botpkg.PaymentSource(tr.Language(), data))
}

func purchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.Clear(callback.From.ID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.self").String(), botpkg.BuyList(tr.Language()))
}

func premiumPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	username := callback.From.Username

	if username != "" {
		session, _ := fsm.UserFSM.Get(callback.From.ID)
		session.State = fsm.StateIdle
		session.MessageID = callback.Message.Message().MessageID
		session.Username = username
		session.Duration = 0
		fsm.UserFSM.Set(callback.From.ID, session)

		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.premium.self").Format("username", username), botpkg.Premium(tr.Language()))
	} else {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.premium.username_self_error").String(), botpkg.AnotherPurchase(tr.Language(), "premium"))
	}
}

func giftPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, giftID int, anonim bool) {
	utils.CallbackAnswer(bot, callback)
	session, _ := fsm.UserFSM.Get(callback.From.ID)
	session.MessageID = callback.Message.Message().MessageID
	session.GiftID = giftID
	session.Anonymous = anonim
	fsm.UserFSM.Set(callback.From.ID, session)

	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.gifts.selected").Format("emojiID", botpkg.Gifts[giftID].EmojiID, "emojiIcon", botpkg.Gifts[giftID].Icon), botpkg.GiftSelected(tr.Language(), giftID, anonim))
}

func giftsPruchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, page ...int) {
	utils.CallbackAnswer(bot, callback)

	if len(page) == 0 {
		session, _ := fsm.UserFSM.Get(callback.From.ID)
		session.State = fsm.StateIdle
		session.MessageID = callback.Message.Message().MessageID
		if callback.From.Username != "" {
			session.Username = callback.From.Username
		}
		fsm.UserFSM.Set(callback.From.ID, session)

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
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.self").Format("MIN_STARS", config.Cfg.MIN_STARS, "MAX_STARS", config.Cfg.MAX_STARS, "username", username), botpkg.AnotherPurchase(tr.Language(), "stars"))
	} else {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.username_self_error").String(), botpkg.AnotherPurchase(tr.Language(), "stars"))
	}
}

func usdPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.slow.usd").String(), botpkg.UzdUsd(tr.Language(), data))
}

func uzsPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	utils.CallbackAnswer(bot, callback)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.slow.uzs").String(), botpkg.UzdUsd(tr.Language(), data))
}

func tonPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	product, amount, username, ok := utils.DivideOrderData(data)
	if !ok || product != "stars" {
		utils.CallbackAnswer(bot, callback, tr.Get("error.not_yet").String())
		return
	}

	utils.CallbackAnswer(bot, callback)
	var amount_ton float32 = float32(amount)
	utils.Edit(bot, callback.Message.Message(), tr.Get("payment.ton").Format("amount", amount, "username", username, "amount_ton", 12, "BANK", config.Cfg.Bank, "order_id", callback.From.ID), botpkg.PayTon(tr.Language(), utils.GenerateTONlink(config.Cfg.Bank, amount_ton, strconv.FormatInt(callback.From.ID, 10))))
}

func adamantPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	product, _, _, ok := utils.DivideOrderData(data)
	if !ok || product != "stars" {
		utils.CallbackAnswer(bot, callback, tr.Get("error.not_yet").String())
		return
	}

	utils.CallbackAnswer(bot, callback, tr.Get("error.balance").String())
}

func cryptomusPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, data string) {
	product, _, _, ok := utils.DivideOrderData(data)
	if !ok || product != "stars" {
		utils.CallbackAnswer(bot, callback, tr.Get("error.not_yet").String())
		return
	}

	utils.CallbackAnswer(bot, callback)
}

func buyGiftsFriend(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.SetState(callback.From.ID, fsm.StateUsernameGifts, callback.Message.Message().MessageID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.gifts.username").String(), botpkg.Cancel(tr.Language()))
}

func buyPremiumFriend(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.SetState(callback.From.ID, fsm.StateUsernamePremium, callback.Message.Message().MessageID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.premium.username").String(), botpkg.Cancel(tr.Language()))
}

func buyStarsFriend(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.SetState(callback.From.ID, fsm.StateUsernameStars, callback.Message.Message().MessageID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.stars.username").String(), botpkg.Cancel(tr.Language()))
}

func nothing(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.Clear(callback.From.ID)
	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.start").String(), botpkg.MainPanel(tr.Language()))
}

func cancelPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer) {
	utils.CallbackAnswer(bot, callback)
	fsm.UserFSM.Clear(callback.From.ID)
	purchase(bot, callback, tr)
}

func premiumDurationPurchase(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, length int) {
	utils.CallbackAnswer(bot, callback)

	username := callback.From.Username
	if savedUsername, ok := fsm.UserFSM.GetUsername(callback.From.ID); ok && savedUsername != "" {
		username = savedUsername
	}

	if username == "" {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.premium.username_self_error").String(), botpkg.AnotherPurchase(tr.Language(), "premium"))
		return
	}

	orderData := utils.GenerateOrderData("premium", length, username)
	utils.Edit(bot, callback.Message.Message(), tr.Payment("source").Format("adamant_balance", currentUserBalance(callback.From.ID)), botpkg.PaymentSource(tr.Language(), orderData))
	fsm.UserFSM.Clear(callback.From.ID)
}

func giftComment(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, giftID int, anonymous bool) {
	utils.CallbackAnswer(bot, callback)

	session, _ := fsm.UserFSM.Get(callback.From.ID)
	session.State = fsm.StateMemo
	session.MessageID = callback.Message.Message().MessageID
	session.GiftID = giftID
	session.Anonymous = anonymous
	fsm.UserFSM.Set(callback.From.ID, session)

	utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.gifts.comment_input").String(), botpkg.Cancel(tr.Language()))
}

func giftWithoutComment(bot *api.Bot, callback *api.CallbackQuery, tr i18n.Localizer, giftID int, anonymous bool) {
	utils.CallbackAnswer(bot, callback)

	username := callback.From.Username
	if savedUsername, ok := fsm.UserFSM.GetUsername(callback.From.ID); ok && savedUsername != "" {
		username = savedUsername
	}

	if username == "" {
		utils.Edit(bot, callback.Message.Message(), tr.Get("menu.buy_list.gifts.username").String(), botpkg.Cancel(tr.Language()))
		return
	}

	session, _ := fsm.UserFSM.Get(callback.From.ID)
	session.GiftID = giftID
	session.Anonymous = anonymous
	fsm.UserFSM.Set(callback.From.ID, session)

	orderData := utils.GenerateOrderData("gifts", giftID, username)
	utils.Edit(bot, callback.Message.Message(), tr.Payment("source").Format("adamant_balance", currentUserBalance(callback.From.ID)), botpkg.PaymentSource(tr.Language(), orderData))
	fsm.UserFSM.Clear(callback.From.ID)
}

func currentUserBalance(userID int64) int64 {
	balance, err := repository.GetUserBalance(context.Background(), userID)
	if err != nil {
		return 0
	}

	return balance
}
