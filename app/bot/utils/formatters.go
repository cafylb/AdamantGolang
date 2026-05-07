package utils

import (
	config "adamant/app/bot/config"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

func RoundUp2(x float64) float64 {
	return math.Ceil(x*100) / 100
}

func GenerateReferal(tgId int) string {
	return fmt.Sprintf("t.me/%s?start=%d", config.Cfg.Username, tgId)
}

func GenerateTONlink(wallet string, amount_ton float64, comment string) string {
	nanoton := int64(math.Round(amount_ton * 1e9))
	return fmt.Sprintf("ton://transfer/%s?amount=%d&text=%s", wallet, nanoton, url.QueryEscape(comment))
}

func GeneratePaymentData(product string, amount int, username string, amount_ton string, system string) string {
	return fmt.Sprintf("%s-%d-%s-%s-%s", product, amount, username, amount_ton, system)
}

func GenerateOrderData(product string, value int, username string) string {
	return fmt.Sprintf("%s-%d-%s", product, value, username)
}

func DivideOrderData(data string) (string, int, string, bool) {
	dataP := strings.SplitN(data, "-", 3)
	if len(dataP) != 3 {
		return "", 0, "", false
	}

	value, err := strconv.Atoi(dataP[1])
	if err != nil {
		return "", 0, "", false
	}

	return dataP[0], value, dataP[2], true
}

func DividePaymentData(data string) (string, int, string, string, string) {
	data = strings.TrimSpace(data)
	if data == "" {
		return "", -1, "", "", ""
	}

	dataP := strings.SplitN(data, "-", 5)
	if len(dataP) != 5 {
		return "", -1, "", "", ""
	}

	amount, err := strconv.Atoi(dataP[1])
	if err != nil {
		return "", -1, "", "", ""
	}
	return dataP[0], amount, dataP[2], dataP[3], dataP[4]
}

func StarsToUsdCoin(amount int) (float64, int) {
	priceCoins := int(math.Round(config.Cfg.Price * float64(amount) * 100))
	return float64(priceCoins) / 100, priceCoins
}

func PremiumToUsdCoin(length string) (float64, int) {
	switch length {
	case "3":
		return 12.99, 1299
	case "6":
		return 16.99, 1699
	case "12":
		return 29.99, 2999
	default:
		return 0.0, 0
	}
}
