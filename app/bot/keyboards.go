package bot

import (
	"fmt"
	"strconv"

	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/utils"

	api "github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

func MainPanel(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.purchase")).WithCallbackData("purchase").WithIconCustomEmojiID("5904462880941545555"),
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.mini_games")).WithCallbackData("nothing").WithIconCustomEmojiID("5773677501825945508"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.support")).WithURL(config.Cfg.Support).WithIconCustomEmojiID("5940433880585605708"),
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.channel")).WithURL(config.Cfg.Channel).WithIconCustomEmojiID("6021418126061605425"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.change_language")).WithCallbackData("choose_language").WithIconCustomEmojiID("5904258298764334001"),
		),
	)
}

func AnotherPurchase(lang string, method string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.another")).WithCallbackData(fmt.Sprintf("buy_%s_friend", method)).WithIconCustomEmojiID("6035084557378654059").WithStyle("primary"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("purchase").WithIconCustomEmojiID("5348414733806484250").WithStyle("danger"),
		),
	)
}

func Cancel(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.cancel")).WithCallbackData("nothing").WithIconCustomEmojiID("5271533904380046720").WithStyle("danger"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("purchase").WithIconCustomEmojiID("5348414733806484250"),
		),
	)
}

func AdminPanel(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.admin.users_staff")).WithCallbackData("users_staff").WithIconCustomEmojiID("5372926953978341366"),
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.admin.broadcast")).WithCallbackData("broadcast_admins").WithIconCustomEmojiID("5465300082628763143"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.admin.list")).WithCallbackData("admins_list").WithIconCustomEmojiID("5470060791883374114"),
		),
	)
}

func BuyList(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.stars")).WithCallbackData("stars_purchase").WithIconCustomEmojiID("6028338546736107668"),
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.premium")).WithCallbackData("premium_purchase").WithIconCustomEmojiID("5879905000972358125"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.gifts")).WithCallbackData("gifts_purchase").WithIconCustomEmojiID("6037175527846975726"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("nothing").WithIconCustomEmojiID("5348414733806484250"),
		),
	)
}

func Support(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.support")).WithURL(config.Cfg.Support).WithIconCustomEmojiID("5940433880585605708"),
		),
	)
}

func Language(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.languages.ru")).WithCallbackData("set_language_ru").WithIconCustomEmojiID("5449408995691341691"),
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.languages.en")).WithCallbackData("set_language_en").WithIconCustomEmojiID("5202021044105257611"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.languages.uz")).WithCallbackData("set_language_uz").WithIconCustomEmojiID("5449829434334912605"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("nothing").WithIconCustomEmojiID("5348414733806484250"),
		),
	)
}

func Profile(lang, WebAppUrl string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.profile")).WithWebApp(tu.WebAppInfo(WebAppUrl)).WithIconCustomEmojiID("5373012449597335010"),
		),
	)
}

func PayTon(lang, url string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.payment.tonkeeper")).WithURL(url).WithIconCustomEmojiID("6037622221625626773"),
		),
	)
}

func PayCryptomus(lang, url string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.payment.cryptomus")).WithURL(url).WithIconCustomEmojiID("6037622221625626773"),
		),
	)
}

func UzdUsd(lang string, data string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.main.support")).WithURL(config.Cfg.Support).WithIconCustomEmojiID("5431376038628171216"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData(fmt.Sprintf("payment_%s", data)).WithIconCustomEmojiID("5348414733806484250"),
		),
	)
}

func Copy(lang, text string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.copy")).WithCopyText(&api.CopyTextButton{Text: text}).WithIconCustomEmojiID("5197269100878907942"),
		),
	)
}

func PaymentSource(lang string, data string) *api.InlineKeyboardMarkup {
	product, value, _, ok := utils.DivideOrderData(data)
	var usdPrice float64
	var priceCoins int
	if !ok {
		product = ""
	}

	switch product {
	case "stars":
		usdPrice, priceCoins = utils.StarsToUsdCoin(value)
	case "premium":
		usdPrice, priceCoins = utils.PremiumToUsdCoin(strconv.Itoa(value))
	case "gifts":
		if value >= 0 && value < len(Gifts) {
			usdPrice = Gifts[value].Price
			priceCoins = int(usdPrice * 100)
		}
	}

	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("TON (%.2f$)", usdPrice)).WithCallbackData(fmt.Sprintf("pay_ton_%s", data)).WithIconCustomEmojiID("5769406891289481208"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("USD").WithCallbackData(fmt.Sprintf("pay_usd_%s", data)).WithIconCustomEmojiID("5927169041595634481"),
			tu.InlineKeyboardButton("UZS").WithCallbackData(fmt.Sprintf("pay_uzs_%s", data)).WithIconCustomEmojiID("5927169041595634481"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("Adamant Balance (%d coins)", priceCoins)).WithCallbackData(fmt.Sprintf("pay_adamant_%s", data)).WithIconCustomEmojiID("5769126056262898415"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("Other Crypto (%.2f$)", usdPrice)).WithCallbackData(fmt.Sprintf("pay_cryptomus_%s", data)).WithIconCustomEmojiID("5346160971192747426"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("purchase").WithIconCustomEmojiID("5348414733806484250"),
		),
	)
}

func GiftList(lang string, page ...int) *api.InlineKeyboardMarkup {
	var totalPages int = (len(Gifts) + 7) / 8
	var curPage int
	if len(page) == 0 {
		curPage = 1
	} else {
		curPage = page[0]
	}
	s := (curPage - 1) * 8

	var keyboard [][]api.InlineKeyboardButton
	var rows [][]api.InlineKeyboardButton
	for i := s; i < s+8 && i < len(Gifts)-1; i += 2 {
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("| %d coins", int(Gifts[i].Price * 100))).WithCallbackData(fmt.Sprintf("gift_purchase_%d_0", i)).WithIconCustomEmojiID(strconv.FormatInt(Gifts[i].EmojiID, 10)),
			tu.InlineKeyboardButton(fmt.Sprintf("| %d coins", int(Gifts[i+1].Price * 100))).WithCallbackData(fmt.Sprintf("gift_purchase_%d_0", i+1)).WithIconCustomEmojiID(strconv.FormatInt(Gifts[i+1].EmojiID, 10)),
		))
	}

	if curPage == totalPages {
		if len(Gifts)%2 == 1 {
			rows = append(rows, tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(fmt.Sprintf("| %d coins", int(Gifts[len(Gifts)-1].Price * 100))).WithCallbackData(fmt.Sprintf("gift_purchase_%d_0", len(Gifts)-1)).WithIconCustomEmojiID(strconv.FormatInt(Gifts[len(Gifts)-1].EmojiID, 10)),
				tu.InlineKeyboardButton("\u200B").WithCallbackData("ignore").WithIconCustomEmojiID(""),
			))
		}
	}

	switch curPage {
	case 1:
		keyboard = append(keyboard, rows...)
		keyboard = append(keyboard, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("%d/%d", curPage, totalPages)).WithCallbackData("ignore"),
			tu.InlineKeyboardButton("\u200B").WithCallbackData(fmt.Sprintf("gifts_purchase_%d", curPage+1)).WithIconCustomEmojiID("5345844853009828446"),
		))
	case totalPages:
		keyboard = append(keyboard, rows...)
		keyboard = append(keyboard, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("\u200B").WithCallbackData(fmt.Sprintf("gifts_purchase_%d", curPage-1)).WithIconCustomEmojiID("5348414733806484250"),
			tu.InlineKeyboardButton(fmt.Sprintf("%d/%d", curPage, totalPages)).WithCallbackData("ignore"),
		))
	default:
		keyboard = append(keyboard, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("%d/%d", curPage, totalPages)).WithCallbackData("ignore"),
		))
		keyboard = append(keyboard, rows...)
		keyboard = append(keyboard, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("\u200B").WithCallbackData(fmt.Sprintf("gifts_purchase_%d", curPage-1)).WithIconCustomEmojiID("5348414733806484250"),
			tu.InlineKeyboardButton("\u200B").WithCallbackData(fmt.Sprintf("gifts_purchase_%d", curPage+1)).WithIconCustomEmojiID("5345844853009828446"),
		))
	}

	keyboard = append(keyboard, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.another")).WithCallbackData("buy_gifts_friend").WithIconCustomEmojiID("6035084557378654059").WithStyle("primary"),
	))
	keyboard = append(keyboard, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("purchase").WithIconCustomEmojiID("5348414733806484250").WithStyle("danger"),
	))

	return tu.InlineKeyboard(keyboard...)
}

func GiftSelected(lang string, giftID int, anonim bool) *api.InlineKeyboardMarkup {
	var anonimS, data string
	if anonim {
		anonimS = "button.gifts.anonim.yes"
		data = fmt.Sprintf("gift_purchase_%d_0", giftID)
	} else {
		anonimS = "button.gifts.anonim.no"
		data = fmt.Sprintf("gift_purchase_%d_1", giftID)
	}
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, anonimS)).WithCallbackData(data).WithIconCustomEmojiID("5303360582706015886"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.gifts.comment.self")).WithCallbackData(fmt.Sprintf("gift_comment_%d_%t", giftID, anonim)).WithIconCustomEmojiID("5883997877172179131"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.gifts.comment.without")).WithCallbackData(fmt.Sprintf("gift_without_comment_%d_%t", giftID, anonim)).WithIconCustomEmojiID("5967355281057779430"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("gifts_purchase").WithIconCustomEmojiID("5348414733806484250").WithStyle("danger"),
		),
	)
}

func Premium(lang string) *api.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.premium.year")).WithCallbackData("premium_purchase_12").WithIconCustomEmojiID("5388849303982716989"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.premium.half")).WithCallbackData("premium_purchase_6").WithIconCustomEmojiID("5388849303982716989"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.premium.three")).WithCallbackData("premium_purchase_3").WithIconCustomEmojiID("5388849303982716989"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.buy_list.another")).WithCallbackData("buy_premium_friend").WithIconCustomEmojiID("6035084557378654059").WithStyle("primary"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.GetFor(lang, "button.utils.back")).WithCallbackData("purchase").WithIconCustomEmojiID("5348414733806484250").WithStyle("danger"),
		),
	)
}
