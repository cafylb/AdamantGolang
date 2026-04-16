package utils

import (
	config "adamant/app/bot/config"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func RoundUp2(x float64) float64 {
	return math.Ceil(x*100) / 100
}

func GenerateReferal(tgId int) string {
	return fmt.Sprintf("t.me/%s?start=%d", config.Cfg.Username, tgId)
}

func GenerateTONlink(wallet string, amount_ton float32, comment string) string {
	var nanoton int = int(amount_ton * float32(math.Pow(10, 9)))
	return fmt.Sprintf("ton://transfer/%s?amount=%d&text=%s", wallet, nanoton, comment)
}

func GeneratePaymentData(product string, amount int, username string, amount_ton string, system string) string {
	return fmt.Sprintf("%s-%d-%s-%s-%s", product, amount, username, amount_ton, system)
}

func DividePaymentData(data string) (string, int, string, string, string) {
	dataP := strings.Split(data, "-")
	amount, err := strconv.Atoi(dataP[1])
	if err != nil {
		return "", -1, "", "", ""
	}	
	return dataP[0], amount, dataP[2], dataP[3], dataP[4]
}