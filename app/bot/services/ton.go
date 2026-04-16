package services

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	i18n "adamant/app/bot/core/i18n"
	repository "adamant/app/bot/database/repository"
	"adamant/app/bot/utils"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/mymmrac/telego"
)

const (
	transactionsUrl = "https://tonapi.io/v2/blockchain/accounts/%s/transactions?limit=20&include_msg=true"
	tonSystem = "TON <tg-emoji emoji-id='5406976471153545018'>&#9786;</tg-emoji>"
)

var (
    starsPattern = regexp.MustCompile(`^stars-\d+-[\w]+-[\d.]+-ton$`)
    giftsPattern = regexp.MustCompile(`^gifts-\d+-[\w]+-[^-]+-ton$`)
    premiumPattern = regexp.MustCompile(`^premium-\d+-[\w]+-ton$`)
)

type TONClient struct {
	db *pgxpool.Pool
	httpClient *http.Client
	pattern *regexp.Regexp
}

type tonAPITransactionsResponse struct {
	Transactions []tonAPITransaction `json:"transactions"`
}

type tonAPITransaction struct {
	Hash  string `json:"hash"`
	InMsg *tonAPIInMessage `json:"in_msg"`
}

type tonAPIInMessage struct {
	Value string `json:"value"`
	Comment string `json:"comment"`
	DecodedBody *tonAPIDecodedMessage `json:"decoded_body"`
}

type tonAPIDecodedMessage struct {
	Text string `json:"text"`
}

var fragmentInitMu sync.Mutex

func NewTONClient(db *pgxpool.Pool) *TONClient {
	return &TONClient{
		db:      db,
		pattern: regexp.MustCompile(`^\d{4,20}-[0-9a-f]{32}-ton$`),
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

func (client *TONClient) Start() {
	if client.pattern == nil {
		client.pattern = regexp.MustCompile()
	}
}

func (client *TONClient) Close() {
	if client.httpClient == nil {
		return
	}

	if transport, ok := client.httpClient.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}

	client.httpClient = nil
}

func (client *TONClient) SuccessUser(bot *api.Bot, purchase repository.Purchase) {
	product, amount, username, amount_ton, system := utils.DividePaymentData(purchase.Data)
	username = html.EscapeString(strings.TrimSpace(username))
	provider := ""
	if system == "ton" {
		provider = tonSystem
	}
	userTr := i18n.ForUser(purchase.UserID)
	text := userTr.Payment("success").Format(
		"provider", provider,
		"product", product,
		"amount", amount,
		"cost", amount_ton,
		"username", username,
	)

	utils.Answer(bot, purchase.UserID, text)
}

func (client *TONClient) CheckTransactions(ctx context.Context) []tonAPITransaction {
	headers := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(config.Cfg.Ton),
		"Cache-Control": "no-cache",
	}

	url := fmt.Sprintf(transactionsUrl, strings.)
}