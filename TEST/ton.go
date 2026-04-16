package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	botpkg "adamant/app/bot"
	config "adamant/app/bot/config"
	i18n "adamant/app/bot/core/i18n"
	repository "adamant/app/bot/database/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/mymmrac/telego"
) 

const (
	tonAPIRequestTimeout           = 8 * time.Second
	tonAPIRetryAttempts            = 1
	tonAPIRetryDelayBase           = 400 * time.Millisecond
	paymentMonitorPollInterval     = 4 * time.Second
	paymentMonitorErrorDelay       = 8 * time.Second
	paidOrdersWorkerPollInterval   = 1 * time.Second
	paidOrdersWorkerErrorDelay     = 4 * time.Second
	orderProcessMaxRetries         = 3
	orderProcessRetryDelay         = 1 * time.Second
	expiredOrdersCleanupInterval   = 120 * time.Second
	completedOrdersCleanupInterval = 15 * time.Second
	processedTxsMax                = 500
	paymentWindow                  = 30 * time.Minute
	underpaymentToleranceTON       = 0.0001

	tonTransactionsURL = "https://tonapi.io/v2/blockchain/accounts/%s/transactions?limit=20&include_msg=true"
	tonProviderHTML    = "TON <tg-emoji emoji-id='5406976471153545018'>&#9786;</tg-emoji>"
) 

type TONClient struct {
	db         *pgxpool.Pool
	httpClient *http.Client
	pattern    *regexp.Regexp
}

type tonAPITransactionsResponse struct {
	Transactions []tonAPITransaction `json:"transactions"`
}

type tonAPITransaction struct {
	Hash  string           `json:"hash"`
	InMsg *tonAPIInMessage `json:"in_msg"`
}

type tonAPIInMessage struct {
	Value       string                `json:"value"`
	Comment     string                `json:"comment"`
	DecodedBody *tonAPIDecodedMessage `json:"decoded_body"`
}

type tonAPIDecodedMessage struct {
	Text string `json:"text"`
}

var fragmentInitMu sync.Mutex

func NewTONClient(db *pgxpool.Pool) *TONClient {
	return &TONClient{
		db: db,
		httpClient: &http.Client{
			Timeout: tonAPIRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

func (client *TONClient) Start() {
	if client.pattern == nil {
		client.pattern = regexp.MustCompile(`^\d{4,20}-[0-9a-f]{32}-ton$`)
	}

	if client.httpClient != nil {
		return
	}

	client.httpClient = &http.Client{
		Timeout: tonAPIRequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
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

	client.httpClient = nil
}

func (client *TONClient) SendStars(ctx context.Context, user string, amount int32, memo string) (string, error) {
	_ = memo

	if err := ensureFragmentService(); err != nil {
		return "", err
	}

	return BuyStarsSimple(ctx, user, int(amount))
}

func (client *TONClient) SuccessUser(bot *api.Bot, purchase repository.Purchase) {
	safeUsername := html.EscapeString(strings.TrimSpace(purchase.Username))
	userTr := i18n.ForUser(purchase.User.TgId)
	text := userTr.Payment("success").Format(
		"provider", tonProviderHTML,
		"amount", purchase.StarsCount,
		"cost", purchase.Amount,
		"username", safeUsername,
	)

	if err := sendHTMLMessage(bot, purchase.User.TgId, text, botpkg.Support(userTr.Language())); err != nil {
		log.Printf("failed to notify user about completed order %s: %v", purchase.OrderID, err)
	}

	for _, adminID := range client.ConfiguredAdminIDs() {
		adminTr := i18n.ForUser(adminID)
		adminText := adminTr.Payment("success").Format(
			"provider", tonProviderHTML,
			"amount", purchase.StarsCount,
			"cost", purchase.Amount,
			"username", safeUsername,
		)

		if err := sendHTMLMessage(bot, adminID, adminText, botpkg.Support(adminTr.Language())); err != nil {
			log.Printf("failed to notify admin %d about completed order %s: %v", adminID, purchase.OrderID, err)
		}
	}
}

func (client *TONClient) FailedUser(bot *api.Bot, purchase repository.Purchase, processErr error, retryCount int32) {
	admins := client.ConfiguredAdminIDs()
	if len(admins) == 0 {
		log.Printf("no admins found to notify about failed order %s", purchase.OrderID)
		return
	}

	escapedError := html.EscapeString(fmt.Sprintf("%T: %v", processErr, processErr))
	safeUsername := html.EscapeString(strings.TrimSpace(purchase.Username))
	tgUserLink := tgLink(purchase.User.TgId, strconv.FormatInt(purchase.User.TgId, 10))
	tgBottomLink := tgLink(purchase.User.TgId, "Open Telegram")

	text := strings.Join([]string{
		"FAILED",
		"",
		fmt.Sprintf("Order ID: <code>%s</code>", html.EscapeString(purchase.OrderID)),
		"Status: <code>failed</code>",
		fmt.Sprintf("Attempts: %d/%d", retryCount, orderProcessMaxRetries),
		fmt.Sprintf("Stars: %d", purchase.StarsCount),
		fmt.Sprintf("Amount: %s", html.EscapeString(purchase.Amount)),
		fmt.Sprintf("Recipient: @%s", safeUsername),
		fmt.Sprintf("Buyer: %s", tgUserLink),
		fmt.Sprintf("Error: <code>%s</code>", escapedError),
		"",
		tgBottomLink,
	}, "\n")

	for _, adminID := range admins {
		if err := sendHTMLMessage(bot, adminID, text, nil); err != nil {
			log.Printf("failed to notify admin %d about failed order %s: %v", adminID, purchase.OrderID, err)
		}
	}
}

func (client *TONClient) CheckTransactions(ctx context.Context) []tonAPITransaction {
	client.Start()

	headers := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(config.Cfg.Ton),
		"Cache-Control": "no-cache",
	}

	url := fmt.Sprintf(tonTransactionsURL, strings.TrimSpace(config.Cfg.Bank))

	for attempt := 0; attempt <= tonAPIRetryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			log.Printf("TonAPI request build failed: %v", err)
			return nil
		}

		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := client.httpClient.Do(req)
		if err != nil {
			if attempt < tonAPIRetryAttempts {
				if !sleepContext(ctx, tonAPIRetryDelayBase*time.Duration(1<<attempt)) {
					return nil
				}
				continue
			}

			log.Printf("TonAPI connection failed: %T: %v", err, err)
			return nil
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			log.Printf("TonAPI body read failed: %v", readErr)
			return nil
		}

		if resp.StatusCode == http.StatusOK {
			var payload tonAPITransactionsResponse
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err != nil {
				log.Printf("TonAPI decode failed: %v", err)
				return nil
			}

			return payload.Transactions
		}

		if resp.StatusCode >= http.StatusInternalServerError {
			if attempt < tonAPIRetryAttempts {
				if !sleepContext(ctx, tonAPIRetryDelayBase*time.Duration(1<<attempt)) {
					return nil
				}
				continue
			}

			log.Printf("TonAPI error: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return nil
		}

		log.Printf("TonAPI error: %d | %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return nil
	}

	return nil
}

func (client *TONClient) PaymentMonitor(ctx context.Context, bot *api.Bot) {
	if client.db == nil {
		log.Println("TON payment monitor stopped: database pool is nil")
		return
	}
	if bot == nil {
		log.Println("TON payment monitor stopped: bot is nil")
		return
	}

	log.Println("TON payment monitor started")

	processed := make(map[string]struct{}, processedTxsMax)
	order := make([]string, 0, processedTxsMax)
	lastCleanup := time.Time{}

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

		if len(order) <= processedTxsMax {
			return
		}

		oldest := order[0]
		order = order[1:]
		delete(processed, oldest)
	}

	for {
		if ctx.Err() != nil {
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("critical panic in TON payment monitor: %v", r)
				}
			}()

			now := time.Now()
			if lastCleanup.IsZero() || now.Sub(lastCleanup) >= expiredOrdersCleanupInterval {
				if err := repository.CleanupExpiredOrders(ctx, client.db); err != nil {
					log.Printf("failed to cleanup expired TON orders: %v", err)
				}
				lastCleanup = now
			}

			transactions := client.CheckTransactions(ctx)
			for _, tx := range transactions {
				txHash := strings.TrimSpace(tx.Hash)
				if txHash != "" {
					if _, exists := processed[txHash]; exists {
						continue
					}
				}

				inMsg := tx.InMsg
				if inMsg == nil {
					markProcessed(txHash)
					continue
				}

				valueNano, err := strconv.ParseInt(strings.TrimSpace(inMsg.Value), 10, 64)
				if err != nil || valueNano <= 0 {
					markProcessed(txHash)
					continue
				}

				memo := strings.TrimSpace(inMsg.Comment)
				if memo == "" && inMsg.DecodedBody != nil {
					memo = strings.TrimSpace(inMsg.DecodedBody.Text)
				}

				if memo == "" || !client.pattern.MatchString(memo) {
					markProcessed(txHash)
					continue
				}

				log.Printf("new TON tx: %.9f TON | memo=%q | hash=%s", float64(valueNano)/1e9, memo, shortHash(txHash))

				purchase, err := repository.GetPurchaseByMemo(ctx, client.db, memo)
				if err != nil {
					log.Printf("failed to load purchase %s: %v", memo, err)
					markProcessed(txHash)
					continue
				}
				if purchase == nil {
					markProcessed(txHash)
					continue
				}

				if purchase.Status != "pending" {
					log.Printf("order %s is already %s", memo, purchase.Status)
					markProcessed(txHash)
					continue
				}

				createdAt := purchase.CreatedAt.UTC()
				if time.Since(createdAt) > paymentWindow {
					log.Printf("order %s found but expired", memo)
					markProcessed(txHash)
					continue
				}

				expectedTON, err := strconv.ParseFloat(strings.TrimSpace(purchase.Amount), 64)
				if err != nil {
					log.Printf("invalid expected TON amount %q for order %s: %v", purchase.Amount, memo, err)
					markProcessed(txHash)
					continue
				}

				receivedTON := float64(valueNano) / 1e9
				log.Printf("payment verification: %.9f TON received, %.9f TON expected for order %s", receivedTON, expectedTON, memo)
				if receivedTON < expectedTON-underpaymentToleranceTON {
					log.Printf("UNDERPAYMENT: %.9f TON received, %.9f TON expected for order %s", receivedTON, expectedTON, memo)
					markProcessed(txHash)
					continue
				}

				if err := repository.UpdatePurchaseStatus(ctx, client.db, purchase.OrderID, "paid"); err != nil {
					log.Printf("failed to mark order %s as paid: %v", purchase.OrderID, err)
					markProcessed(txHash)
					continue
				}

				log.Printf("order %s marked as paid and queued for sequential processing", memo)
				markProcessed(txHash)
			}
		}()

		if !sleepContext(ctx, paymentMonitorPollInterval) {
			return
		}
	}
}

func (client *TONClient) PaidOrdersWorker(ctx context.Context, bot *api.Bot) {
	if client.db == nil {
		log.Println("paid orders worker stopped: database pool is nil")
		return
	}
	if bot == nil {
		log.Println("paid orders worker stopped: bot is nil")
		return
	}

	log.Println("paid orders worker started")

	if err := repository.ResetProcessingPurchases(ctx, client.db); err != nil {
		log.Printf("failed to reset processing purchases: %v", err)
	}

	for {
		if ctx.Err() != nil {
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("critical panic in paid orders worker: %v", r)
				}
			}()

			purchase, err := repository.ClaimNextPaidPurchase(ctx, client.db)
			if err != nil {
				log.Printf("critical error in paid orders worker: %v", err)
				sleepContext(ctx, paidOrdersWorkerErrorDelay)
				return
			}
			if purchase == nil {
				sleepContext(ctx, paidOrdersWorkerPollInterval)
				return
			}

			log.Printf("processing paid order %s: %d stars to @%s", purchase.OrderID, purchase.StarsCount, purchase.Username)

			txHash, err := client.SendStars(ctx, purchase.Username, purchase.StarsCount, purchase.OrderID)
			if err == nil {
				client.SuccessUser(bot, *purchase)
				if statusErr := repository.UpdatePurchaseStatus(ctx, client.db, purchase.OrderID, "completed"); statusErr != nil {
					log.Printf("failed to mark order %s as completed: %v", purchase.OrderID, statusErr)
				}
				log.Printf("order %s completed. Sent %d stars to %s | tx=%s", purchase.OrderID, purchase.StarsCount, purchase.Username, txHash)
				return
			}

			log.Printf("order processing failed for %s: %T: %v", purchase.OrderID, err, err)

			retryCount, retryErr := repository.IncrementPurchaseRetryCount(ctx, client.db, purchase.OrderID)
			if retryErr != nil {
				log.Printf("failed to increment retry count for %s: %v", purchase.OrderID, retryErr)
				sleepContext(ctx, paidOrdersWorkerErrorDelay)
				return
			}

			if retryCount >= orderProcessMaxRetries {
				if statusErr := repository.UpdatePurchaseStatus(ctx, client.db, purchase.OrderID, "failed"); statusErr != nil {
					log.Printf("failed to mark order %s as failed: %v", purchase.OrderID, statusErr)
				}

				client.FailedUser(bot, *purchase, err, retryCount)

				userTr := i18n.ForUser(purchase.User.TgId)
				if notifyErr := sendHTMLMessage(bot, purchase.User.TgId, userTr.Error("fragment").String(), botpkg.Support(userTr.Language())); notifyErr != nil {
					log.Printf("failed to notify user about failed order %s: %v", purchase.OrderID, notifyErr)
				}

				log.Printf("order %s moved to failed after %d/%d attempts", purchase.OrderID, retryCount, orderProcessMaxRetries)
				return
			}

			if statusErr := repository.UpdatePurchaseStatus(ctx, client.db, purchase.OrderID, "paid"); statusErr != nil {
				log.Printf("failed to return order %s back to paid state: %v", purchase.OrderID, statusErr)
			}

			userTr := i18n.ForUser(purchase.User.TgId)
			retryText := userTr.Error("fragment_retry").Format(
				"attempt", retryCount,
				"max_attempts", orderProcessMaxRetries,
			)
			if notifyErr := sendHTMLMessage(bot, purchase.User.TgId, retryText, botpkg.Support(userTr.Language())); notifyErr != nil {
				log.Printf("failed to notify user about retry for order %s: %v", purchase.OrderID, notifyErr)
			}

			sleepContext(ctx, orderProcessRetryDelay)
		}()
	}
}

func (client *TONClient) ConfiguredAdminIDs() []int64 {
	if config.Cfg.Admin == 0 {
		return nil
	}
	return []int64{config.Cfg.Admin}
}

func (client *TONClient) StartWorkers(ctx context.Context, bot *api.Bot) {
	go client.PaymentMonitor(ctx, bot)
	go client.PaidOrdersWorker(ctx, bot)
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

func sendHTMLMessage(bot *api.Bot, chatID int64, text string, keyboard *api.InlineKeyboardMarkup) error {
	if bot == nil {
		return fmt.Errorf("bot is nil")
	}

	params := &api.SendMessageParams{
		ChatID:             api.ChatID{ID: chatID},
		Text:               text,
		ParseMode:          api.ModeHTML,
		LinkPreviewOptions: &api.LinkPreviewOptions{IsDisabled: true},
	}

	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := bot.SendMessage(context.Background(), params)
	return err
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func tgLink(userID int64, label string) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, html.EscapeString(label))
}

func shortHash(txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if len(txHash) <= 16 {
		return txHash
	}
	return txHash[:16] + "..."
}

var TONService *TONClient

func InitTON(db *pgxpool.Pool) error {
	if db == nil {
		return fmt.Errorf("database pool is nil")
	}

	TONService = NewTONClient(db)
	return nil
}
