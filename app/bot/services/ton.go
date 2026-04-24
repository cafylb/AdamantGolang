package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/utils"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/mymmrac/telego"
)

const (
	tonTransactionsURL      = "https://tonapi.io/v2/blockchain/accounts/%s/transactions?limit=20&include_msg=true"
	tonSystem               = "TON <tg-emoji emoji-id='5406976471153545018'>&#9786;</tg-emoji>"
	tonRequestTimeout       = 8 * time.Second
	tonPollInterval         = 10 * time.Second
	tonPaymentWindow        = 30 * time.Minute
	tonUnderpaymentDeltaTON = 0.0001
	tonProcessedLimit       = 512
)

var (
	ErrTONClient          = errors.New("ton client error")
	ErrTONInvalidMemo     = errors.New("ton memo is invalid")
	ErrTONPurchaseMissing = errors.New("ton purchase is missing")
	ErrTONUnderpaid       = errors.New("ton amount is below expected")
)

type TONClient struct {
	db *pgxpool.Pool

	httpClient *http.Client

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

type tonAPITransactionsResponse struct {
	Transactions []tonAPITransaction `json:"transactions"`
}

type tonAPITransaction struct {
	Hash  string           `json:"hash"`
	InMsg *tonAPIInMessage `json:"in_msg"`
}

type tonAPIInMessage struct {
	Value       tonAPINumericString   `json:"value"`
	Comment     string                `json:"comment"`
	DecodedBody *tonAPIDecodedMessage `json:"decoded_body"`
}

type tonAPIDecodedMessage struct {
	Text string `json:"text"`
}

type tonPurchase struct {
	UserID    int64
	Data      string
	Complete  bool
	CreatedAt time.Time
}

type paymentData struct {
	Product   string
	Amount    int
	Username  string
	AmountTON float64
	System    string
}

var fragmentInitMu sync.Mutex
var giftInitMu sync.Mutex

func NewTONClient(db *pgxpool.Pool) *TONClient {
	return &TONClient{
		db: db,
		httpClient: &http.Client{
			Timeout: tonRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

func (client *TONClient) Close() {
	client.mu.Lock()
	cancel := client.cancel
	done := client.done
	client.cancel = nil
	client.done = nil
	client.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	if client.httpClient != nil {
		if transport, ok := client.httpClient.Transport.(interface{ CloseIdleConnections() }); ok {
			transport.CloseIdleConnections()
		}
	}
}

func (client *TONClient) StartWorkers(ctx context.Context, bot *api.Bot) {
	client.mu.Lock()
	if client.done != nil {
		client.mu.Unlock()
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	client.cancel = cancel
	client.done = done
	client.mu.Unlock()

	go func() {
		defer close(done)
		client.PaymentMonitor(runCtx, bot)
	}()
}

func (client *TONClient) SuccessUser(bot *api.Bot, purchase tonPurchase, payment paymentData) {
	if bot == nil {
		return
	}

	username := html.EscapeString(strings.TrimSpace(payment.Username))
	userTr := i18n.ForUser(purchase.UserID)
	text := userTr.Payment("success").Format(
		"provider", tonSystem,
		"product", payment.Product,
		"amount", payment.Amount,
		"cost", trimFloat(payment.AmountTON, 6),
		"username", username,
	)

	utils.Answer(bot, purchase.UserID, text)
}

func (client *TONClient) CheckTransactions(ctx context.Context) []tonAPITransaction {
	if client.httpClient == nil {
		return nil
	}

	bank := strings.TrimSpace(config.Cfg.Bank)
	apiKey := strings.TrimSpace(config.Cfg.Ton)
	if bank == "" || apiKey == "" {
		log.Printf("TON monitor is not configured: BANK or TON_CONSOLE is empty")
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(tonTransactionsURL, bank), nil)
	if err != nil {
		log.Printf("failed to build TonAPI request: %v", err)
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		log.Printf("failed to fetch TonAPI transactions: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("TonAPI returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return nil
	}

	var payload tonAPITransactionsResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		log.Printf("failed to decode TonAPI response: %v", err)
		return nil
	}

	return payload.Transactions
}

func (client *TONClient) PaymentMonitor(ctx context.Context, bot *api.Bot) {
	if client.db == nil {
		log.Println("TON payment monitor stopped: database pool is nil")
		return
	}

	log.Println("TON payment monitor started")

	processed := make(map[string]struct{}, tonProcessedLimit)
	order := make([]string, 0, tonProcessedLimit)

	markProcessed := func(txHash string) {
		txHash = strings.TrimSpace(txHash)
		if txHash == "" {
			return
		}
		if _, exists := processed[txHash]; exists {
			return
		}

		processed[txHash] = struct{}{}
		order = append(order, txHash)
		if len(order) <= tonProcessedLimit {
			return
		}

		oldest := order[0]
		order = order[1:]
		delete(processed, oldest)
	}

	ticker := time.NewTicker(tonPollInterval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}

		for _, tx := range client.CheckTransactions(ctx) {
			txHash := strings.TrimSpace(tx.Hash)
			if _, exists := processed[txHash]; exists {
				continue
			}

			retry, err := client.handleTransaction(ctx, tx, bot)
			if err != nil {
				log.Printf("TON transaction %s failed: %v", shortHash(txHash), err)
			}
			if !retry {
				markProcessed(txHash)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (client *TONClient) handleTransaction(ctx context.Context, tx tonAPITransaction, bot *api.Bot) (bool, error) {
	txHash := strings.TrimSpace(tx.Hash)
	inMsg := tx.InMsg
	if inMsg == nil {
		log.Printf("TON skip %s: incoming message is empty", shortHash(txHash))
		return false, nil
	}

	memo := extractTONMemo(inMsg)
	payment, err := parsePaymentData(memo)
	if err != nil {
		log.Printf("TON skip %s: invalid memo %q", shortHash(txHash), memo)
		return false, nil
	}
	if payment.System != "ton" {
		log.Printf("TON skip %s: memo system is %q, expected ton", shortHash(txHash), payment.System)
		return false, nil
	}

	valueNano, err := inMsg.Value.Int64()
	if err != nil || valueNano <= 0 {
		log.Printf("TON skip %s: invalid incoming value %q", shortHash(txHash), inMsg.Value.String())
		return false, fmt.Errorf("%w: invalid incoming value", ErrTONClient)
	}

	log.Printf(
		"TON accept %s: memo=%q | product=%s | user=%s | expected=%.9f TON | received=%.9f TON",
		shortHash(txHash),
		memo,
		payment.Product,
		payment.Username,
		payment.AmountTON,
		float64(valueNano)/1e9,
	)

	purchase, err := client.getPendingPurchase(ctx, payment.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("TON skip %s: purchase not found for memo %q", shortHash(txHash), payment.String())
			return false, nil
		}
		return true, err
	}

	if time.Since(purchase.CreatedAt.UTC()) > tonPaymentWindow {
		log.Printf("TON skip %s: purchase expired | created_at=%s | memo=%q", shortHash(txHash), purchase.CreatedAt.UTC().Format(time.RFC3339), purchase.Data)
		return false, nil
	}

	receivedTON := float64(valueNano) / 1e9
	if receivedTON < payment.AmountTON-tonUnderpaymentDeltaTON {
		log.Printf(
			"TON skip %s: underpaid | received=%.9f TON | expected=%.9f TON | memo=%q",
			shortHash(txHash),
			receivedTON,
			payment.AmountTON,
			payment.String(),
		)
		return false, fmt.Errorf(
			"%w: got %.9f TON, expected %.9f TON",
			ErrTONUnderpaid,
			receivedTON,
			payment.AmountTON,
		)
	}

	deliveryRef, err := client.fulfillPurchase(ctx, payment)
	if err != nil {
		log.Printf("TON accept %s: payment matched but fulfillment failed for memo %q: %v", shortHash(txHash), payment.String(), err)
		return true, err
	}

	if err := client.completePurchase(ctx, purchase); err != nil {
		log.Printf("purchase %s delivered (%s) but db update failed: %v", purchase.Data, deliveryRef, err)
		return false, nil
	}

	log.Printf(
		"TON complete %s: %s | %.9f TON | %s",
		shortHash(txHash),
		purchase.Data,
		receivedTON,
		deliveryRef,
	)
	client.SuccessUser(bot, *purchase, payment)

	return false, nil
}

func (client *TONClient) fulfillPurchase(ctx context.Context, payment paymentData) (string, error) {
	switch payment.Product {
	case "stars":
		if err := ensureFragmentService(); err != nil {
			return "", err
		}

		txHash, err := BuyStarsSimple(ctx, payment.Username, payment.Amount)
		if err != nil {
			return "", err
		}
		return "fragment:" + txHash, nil
	case "gifts":
		if err := ensureGiftService(); err != nil {
			return "", err
		}

		ok, nft, _, err := GiftService.RandomTransferByUsername(ctx, payment.Username)
		if err != nil {
			return "", err
		}
		if !ok || nft == nil {
			return "", fmt.Errorf("gift transfer returned no result")
		}
		return "gift:" + nft.Link, nil
	case "premium":
		if err := ensureGiftService(); err != nil {
			return "", err
		}

		if ok, err := GiftService.SendPremiumByUsername(ctx, payment.Username, payment.Amount, ""); err != nil {
			return "", err
		} else if !ok {
			return "", fmt.Errorf("premium purchase was not completed")
		}

		return fmt.Sprintf("premium:%d:%s", payment.Amount, payment.Username), nil
	default:
		return "", fmt.Errorf("%w: unsupported product %q", ErrTONClient, payment.Product)
	}
}

func (client *TONClient) getPendingPurchase(ctx context.Context, memo string) (*tonPurchase, error) {
	row := client.db.QueryRow(ctx, `
		SELECT tg_id, data, complete, created_at
		FROM purchases
		WHERE data = $1 AND complete = FALSE
		ORDER BY created_at ASC
		LIMIT 1
	`, memo)

	var purchase tonPurchase
	if err := row.Scan(&purchase.UserID, &purchase.Data, &purchase.Complete, &purchase.CreatedAt); err != nil {
		return nil, err
	}

	return &purchase, nil
}

func (client *TONClient) completePurchase(ctx context.Context, purchase *tonPurchase) error {
	if purchase == nil {
		return fmt.Errorf("%w: purchase is nil", ErrTONClient)
	}

	tag, err := client.db.Exec(ctx, `
		UPDATE purchases
		SET complete = TRUE
		WHERE tg_id = $1 AND data = $2 AND complete = FALSE
	`, purchase.UserID, purchase.Data)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func parsePaymentData(data string) (paymentData, error) {
	data = strings.TrimSpace(data)
	if data == "" || strings.Count(data, "-") < 4 {
		return paymentData{}, ErrTONInvalidMemo
	}

	product, amount, username, amountTON, system := utils.DividePaymentData(data)
	if product == "" || amount <= 0 || username == "" || amountTON == "" || system == "" {
		return paymentData{}, ErrTONInvalidMemo
	}

	valueTON, err := strconv.ParseFloat(strings.TrimSpace(amountTON), 64)
	if err != nil || valueTON <= 0 {
		return paymentData{}, ErrTONInvalidMemo
	}

	return paymentData{
		Product:   strings.TrimSpace(product),
		Amount:    amount,
		Username:  strings.TrimPrefix(strings.TrimSpace(username), "@"),
		AmountTON: valueTON,
		System:    strings.TrimSpace(system),
	}, nil
}

func (payment paymentData) String() string {
	return utils.GeneratePaymentData(
		payment.Product,
		payment.Amount,
		payment.Username,
		trimFloat(payment.AmountTON, 9),
		payment.System,
	)
}

func extractTONMemo(message *tonAPIInMessage) string {
	if message == nil {
		return ""
	}

	memo := strings.TrimSpace(message.Comment)
	if memo != "" {
		return memo
	}
	if message.DecodedBody != nil {
		return strings.TrimSpace(message.DecodedBody.Text)
	}
	return ""
}

func ensureFragmentService() error {
	if FragmentService != nil {
		return nil
	}

	fragmentInitMu.Lock()
	defer fragmentInitMu.Unlock()

	if FragmentService != nil {
		return nil
	}

	return InitFragment()
}

func ensureGiftService() error {
	if GiftService != nil {
		return nil
	}

	giftInitMu.Lock()
	defer giftInitMu.Unlock()

	if GiftService != nil {
		return nil
	}

	return InitGiftManager()
}

func trimFloat(value float64, digits int) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func shortHash(txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if len(txHash) <= 16 {
		return txHash
	}
	return txHash[:16] + "..."
}

type tonAPINumericString string

func (value *tonAPINumericString) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 || string(data) == "null" {
		*value = ""
		return nil
	}

	if data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*value = tonAPINumericString(strings.TrimSpace(str))
		return nil
	}

	*value = tonAPINumericString(string(data))
	return nil
}

func (value tonAPINumericString) String() string {
	return strings.TrimSpace(string(value))
}

func (value tonAPINumericString) Int64() (int64, error) {
	raw := value.String()
	if raw == "" {
		return 0, fmt.Errorf("empty numeric value")
	}

	return strconv.ParseInt(raw, 10, 64)
}

var TONService *TONClient

func InitTON(db *pgxpool.Pool) error {
	if db == nil {
		return fmt.Errorf("database pool is nil")
	}

	TONService = NewTONClient(db)
	return nil
}
