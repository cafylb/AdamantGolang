package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	config "adamant/app/bot/config"
	"adamant/app/bot/utils"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testTONTransactionsURL = "https://tonapi.io/v2/blockchain/accounts/%s/transactions?limit=20&include_msg=true"
	testTONTimeout         = 8 * time.Second
	testTONPollInterval    = 10 * time.Second
	testTONWindow          = 30 * time.Minute
)

var (
	ErrTestTONInvalidMemo = errors.New("test ton memo is invalid")
)

type TONClient struct {
	db         *pgxpool.Pool
	httpClient *http.Client
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

type paymentData struct {
	Product   string
	Amount    int
	Username  string
	AmountTON float64
	System    string
}

type purchaseRow struct {
	UserID    int64
	Data      string
	Complete  bool
	CreatedAt time.Time
}

func NewTONClient(db *pgxpool.Pool) *TONClient {
	return &TONClient{
		db: db,
		httpClient: &http.Client{
			Timeout: testTONTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

func (client *TONClient) Close() {
	if client.httpClient == nil {
		return
	}
	if transport, ok := client.httpClient.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

func (client *TONClient) CheckTransactions(ctx context.Context) []tonAPITransaction {
	if client.httpClient == nil {
		return nil
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(testTONTransactionsURL, strings.TrimSpace(config.Cfg.Bank)),
		nil,
	)
	if err != nil {
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(config.Cfg.Ton))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	var payload tonAPITransactionsResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}

	return payload.Transactions
}

func (client *TONClient) PaymentMonitor(ctx context.Context) error {
	if client.db == nil {
		return fmt.Errorf("database pool is nil")
	}

	ticker := time.NewTicker(testTONPollInterval)
	defer ticker.Stop()

	for {
		if err := client.pollOnce(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (client *TONClient) pollOnce(ctx context.Context) error {
	for _, tx := range client.CheckTransactions(ctx) {
		if tx.InMsg == nil {
			continue
		}

		payment, err := parsePaymentData(extractMemo(tx.InMsg))
		if err != nil || payment.System != "ton" {
			continue
		}

		purchase, err := client.findPendingPurchase(ctx, payment.String())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}

		if time.Since(purchase.CreatedAt.UTC()) > testTONWindow {
			continue
		}

		if payment.Product != "stars" {
			continue
		}

		if _, err := BuyStarsSimple(ctx, payment.Username, payment.Amount); err != nil {
			return err
		}

		if err := client.completePurchase(ctx, purchase); err != nil {
			return err
		}
	}

	return nil
}

func (client *TONClient) findPendingPurchase(ctx context.Context, memo string) (*purchaseRow, error) {
	row := client.db.QueryRow(ctx, `
		SELECT tg_id, data, complete, created_at
		FROM purchases
		WHERE data = $1 AND complete = FALSE
		ORDER BY created_at ASC
		LIMIT 1
	`, memo)

	var purchase purchaseRow
	if err := row.Scan(&purchase.UserID, &purchase.Data, &purchase.Complete, &purchase.CreatedAt); err != nil {
		return nil, err
	}

	return &purchase, nil
}

func (client *TONClient) completePurchase(ctx context.Context, purchase *purchaseRow) error {
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
		return paymentData{}, ErrTestTONInvalidMemo
	}

	product, amount, username, amountTON, system := utils.DividePaymentData(data)
	if product == "" || amount <= 0 || username == "" || amountTON == "" || system == "" {
		return paymentData{}, ErrTestTONInvalidMemo
	}

	valueTON, err := strconv.ParseFloat(amountTON, 64)
	if err != nil || valueTON <= 0 {
		return paymentData{}, ErrTestTONInvalidMemo
	}

	return paymentData{
		Product:   product,
		Amount:    amount,
		Username:  strings.TrimPrefix(username, "@"),
		AmountTON: valueTON,
		System:    system,
	}, nil
}

func (payment paymentData) String() string {
	return utils.GeneratePaymentData(
		payment.Product,
		payment.Amount,
		payment.Username,
		strconv.FormatFloat(payment.AmountTON, 'f', -1, 64),
		payment.System,
	)
}

func extractMemo(message *tonAPIInMessage) string {
	if message == nil {
		return ""
	}
	if memo := strings.TrimSpace(message.Comment); memo != "" {
		return memo
	}
	if message.DecodedBody != nil {
		return strings.TrimSpace(message.DecodedBody.Text)
	}
	return ""
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
