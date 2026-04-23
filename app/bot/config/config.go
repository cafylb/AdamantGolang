package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Token     string
	Username  string
	MIN_STARS int32
	MAX_STARS int32
	Price     float64

	DB          string
	FSM_STORAGE string
	REDIS_URL   string

	Bank          string
	WalletSeed    string
	WalletVersion string
	Ton           string

	FragmentCookies string
	FragmentHash    string

	CryptomusMerchantId string
	CryptomusPayment    string
	CryptomusPayout     string
	CryptomusWebhookUrl string

	Number  string
	ApiId   string
	ApiHash string
	Admin   int64

	Gifts         string
	Adamant       string
	Channel       string
	Support       string
}

func Load() *Config {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Failed to load .env: ", err)
	}

	return &Config{
		Token:               get("TOKEN"),
		Username:            get("BOT_USERNAME"),
		MIN_STARS:           getInt32("MIN_STARS"),
		MAX_STARS:           getInt32("MAX_STARS"),
		Price:               getFloat64("PRICE"),
		DB:                  get("DB"),
		FSM_STORAGE:         getDefault("FSM_STORAGE", "redis"),
		REDIS_URL:           getDefault("REDIS_URL", "redis://127.0.0.1:6379/0"),
		Bank:                get("BANK"),
		WalletSeed:          get("WALLET_SEED"),
		WalletVersion:       get("WALLET_VERSION"),
		Ton:                 get("TON_CONSOLE"),
		FragmentCookies:     get("FRAGMENT_COOKIES"),
		FragmentHash:        get("FRAGMENT_HASH"),
		CryptomusMerchantId: get("CRYPTOMUS_MERCHANT_ID"),
		CryptomusPayment:    get("CRYPTOMUS_MERCHANT_PAYMENT"),
		CryptomusPayout:     get("CRYPTOMUS_PAYOUT"),
		CryptomusWebhookUrl: getDefault("CRYPTOMUS_WEBHOOK_URL", ""),
		Number:              get("NUMBER"),
		ApiId:               get("API_ID"),
		ApiHash:             get("API_HASH"),
		Admin:               getInt64("ADMIN"),
		Gifts:               get("GIFTS"),
		Adamant:             get("ADAMANT"),
		Channel:             get("CHANNEL"),
		Support:             get("SUPPORT"),
	}
}

func get(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal(key, " is empty")
	}
	return value
}

func getDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getInt32(key string) int32 {
	value := get(key)

	num, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		log.Fatal("invalid ", key, ": ", err)
	}

	return int32(num)
}

func getInt64(key string) int64 {
	value := get(key)

	num, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		log.Fatal("invalid ", key, ": ", err)
	}

	return num
}

func getFloat64(key string) float64 {
	value := get(key)

	num, err := strconv.ParseFloat(value, 32)
	if err != nil {
		log.Fatal("invalid ", key, ": ", err)
	}

	return float64(num)
}

var Cfg = Load()
