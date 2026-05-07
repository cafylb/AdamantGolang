package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	config "adamant/app/bot/config"

	tonaddr "github.com/xssnick/tonutils-go/address"
	liteclient "github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	tonapi "github.com/xssnick/tonutils-go/ton"
	tonwallet "github.com/xssnick/tonutils-go/ton/wallet"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

var (
	ErrFragmentClient       = errors.New("fragment client error")
	ErrFragmentAuth         = errors.New("fragment auth error")
	ErrFragmentTimeout      = errors.New("fragment timeout error")
	ErrFragmentBalance      = errors.New("fragment balance error")
	ErrFragmentVerification = errors.New("fragment verification error")
	ErrFragmentServer       = errors.New("fragment server error")
)

const (
	fragmentAPIURL     = "https://fragment.com/api"
	fragmentHomeURL    = "https://fragment.com/"
	fragmentLiteConfig = "https://ton-blockchain.github.io/global.config.json"
	tonAccountURL      = "https://tonapi.io/v2/accounts/%s"
	buyFeeBufferTON    = 0.001
	defaultHTTPTimeout = 20 * time.Second
)

var (
	requiredFragmentCookies = []string{"stel_ssid", "stel_token", "stel_dt", "stel_ton_token"}

	hashPatterns = []*regexp.Regexp{
		regexp.MustCompile(`apiUrl":"\\?/api\?hash=([a-f0-9]+)`),
		regexp.MustCompile(`api\?hash=([a-f0-9]+)`),
	}

	walletInitPattern = regexp.MustCompile(`Wallet\.init\((\{.*?\})\);`)
)

var fragmentHeaders = map[string]string{
	"accept":           "application/json, text/javascript, */*; q=0.01",
	"content-type":     "application/x-www-form-urlencoded; charset=UTF-8",
	"origin":           "https://fragment.com",
	"referer":          "https://fragment.com/",
	"user-agent":       "Mozilla/5.0",
	"x-requested-with": "XMLHttpRequest",
}

type FragmentPurchaseResult struct {
	Success   bool
	TxHash    string
	AmountTON float64
	Recipient string
}

type Fragment struct {
	mu sync.Mutex

	seed    string
	tonKey  string
	walletV string
	walletA string
	hashV   string

	walletConnected *bool
	cookies         map[string]string

	httpClient *http.Client
	liteClient *liteclient.ConnectionPool
	tonClient  tonapi.APIClientWrapped
	wallet     *tonwallet.Wallet
}

type fragmentTxMessage struct {
	Address string
	Amount  string
	Payload string
}

func NewFragmentClient() (*Fragment, error) {
	seed, err := normalizeSeed(config.Cfg.WalletSeed)
	if err != nil {
		return nil, err
	}

	return &Fragment{
		seed:       seed,
		tonKey:     strings.TrimSpace(config.Cfg.Ton),
		walletV:    strings.ToUpper(strings.TrimSpace(config.Cfg.WalletVersion)),
		walletA:    strings.TrimSpace(config.Cfg.Bank),
		hashV:      strings.TrimSpace(config.Cfg.FragmentHash),
		cookies:    parseCookies(strings.TrimSpace(config.Cfg.FragmentCookies)),
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}, nil
}

func (client *Fragment) Start(ctx context.Context) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	if client.liteClient != nil && client.tonClient != nil && client.wallet != nil {
		return nil
	}

	pool := liteclient.NewConnectionPool()

	cfg, err := liteclient.GetConfigFromUrl(ctx, fragmentLiteConfig)
	if err != nil {
		pool.Stop()
		return fmt.Errorf("%w: failed to fetch TON lite config: %v", ErrFragmentClient, err)
	}

	if err := pool.AddConnectionsFromConfig(ctx, cfg); err != nil {
		pool.Stop()
		return fmt.Errorf("%w: failed to connect TON lite servers: %v", ErrFragmentClient, err)
	}

	api := tonapi.NewAPIClient(pool, tonapi.ProofCheckPolicyFast).WithRetry()
	api.SetTrustedBlockFromConfig(cfg)

	versionCfg, err := walletVersionConfig(client.walletV)
	if err != nil {
		pool.Stop()
		return err
	}

	wallet, err := tonwallet.FromSeedWithOptions(api, strings.Fields(client.seed), versionCfg)
	if err != nil {
		pool.Stop()
		return fmt.Errorf("%w: failed to init TON wallet from seed: %v", ErrFragmentAuth, err)
	}

	if client.walletA != "" {
		cfgAddr, err := tonaddr.ParseAddr(client.walletA)
		if err != nil {
			pool.Stop()
			return fmt.Errorf("%w: invalid BANK address: %v", ErrFragmentAuth, err)
		}

		if !cfgAddr.Equals(wallet.WalletAddress()) {
			pool.Stop()
			return fmt.Errorf(
				"%w: BANK address %s does not match wallet address from seed %s",
				ErrFragmentAuth,
				cfgAddr.StringRaw(),
				wallet.WalletAddress().StringRaw(),
			)
		}
	}

	client.liteClient = pool
	client.tonClient = api
	client.wallet = wallet
	return nil
}

func (client *Fragment) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.liteClient != nil {
		client.liteClient.Stop()
		client.liteClient = nil
	}

	client.tonClient = nil
	client.wallet = nil
	return nil
}

func (client *Fragment) GetWalletBalance(ctx context.Context) (float64, error) {
	if client.walletA == "" {
		return 0, fmt.Errorf("%w: BANK не задан", ErrFragmentAuth)
	}
	if client.tonKey == "" {
		return 0, fmt.Errorf("%w: TON API KEY не задан", ErrFragmentAuth)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(tonAccountURL, client.walletA), nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}

	req.Header.Set("Authorization", "Bearer "+client.tonKey)
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrFragmentTimeout, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, fmt.Errorf("%w: неправильный TON API KEY", ErrFragmentAuth)
	}
	if resp.StatusCode >= 500 {
		return 0, fmt.Errorf("%w: сервера TON CONSOLE выдали ошибку на своей стороне", ErrFragmentServer)
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("%w: tonapi status %d", ErrFragmentClient, resp.StatusCode)
	}

	var payload struct {
		Balance json.Number `json:"balance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("%w: decode balance: %v", ErrFragmentClient, err)
	}

	nano, err := payload.Balance.Int64()
	if err != nil {
		return 0, fmt.Errorf("%w: invalid balance", ErrFragmentClient)
	}

	return float64(nano) / 1e9, nil
}

func (client *Fragment) BuyStars(ctx context.Context, user string, amount int) (*FragmentPurchaseResult, error) {
	if err := client.Start(ctx); err != nil {
		return nil, err
	}

	initPayload, err := client.FragmentAPI(ctx, map[string]string{
		"recipient": user,
		"quantity":  strconv.Itoa(amount),
		"method":    "initBuyStarsRequest",
	})
	if err != nil {
		return nil, err
	}

	reqID := anyToString(initPayload["req_id"])
	if reqID == "" {
		return nil, fmt.Errorf("%w: Fragment не вернул req_id", ErrFragmentClient)
	}

	linkPayload, err := client.FragmentAPI(ctx, map[string]string{
		"transaction": "1",
		"id":          reqID,
		"show_sender": "0",
		"method":      "getBuyStarsLink",
	})
	if err != nil {
		return nil, err
	}

	msg, err := extractTransactionMessage(linkPayload)
	if err != nil {
		return nil, err
	}

	amountNano, err := strconv.ParseUint(msg.Amount, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid nano amount from Fragment: %v", ErrFragmentClient, err)
	}

	amountTON := float64(amountNano) / 1e9
	requiredTON := amountTON + buyFeeBufferTON

	balanceTON, err := client.GetWalletBalance(ctx)
	if err != nil {
		return nil, err
	}
	if balanceTON < requiredTON {
		return nil, fmt.Errorf(
			"%w: недостаточно средств. Нужно ~%.6f TON, доступно %.6f TON",
			ErrFragmentBalance,
			requiredTON,
			balanceTON,
		)
	}

	txHash, err := client.SendTON(ctx, msg.Address, msg.Amount, msg.Payload)
	if err != nil {
		return nil, err
	}

	return &FragmentPurchaseResult{
		Success:   true,
		TxHash:    txHash,
		AmountTON: requiredTON,
		Recipient: user,
	}, nil
}

func (client *Fragment) CheckUsername(ctx context.Context, username string) (string, error) {
	payload, err := client.FragmentAPI(ctx, map[string]string{
		"query":    username,
		"method":   "searchStarsRecipient",
		"quantity": "",
	})
	if err != nil {
		return "", err
	}

	found, _ := payload["found"].(map[string]any)
	if found == nil {
		return "", nil
	}

	return anyToString(found["recipient"]), nil
}

func (client *Fragment) CheckPremium(ctx context.Context, username string, months int) (bool, string, error) {
	if client == nil {
		return false, "", fmt.Errorf("%w: fragment client not initialized", ErrFragmentClient)
	}
	if err := client.IsFragmentConnected(); err != nil {
		return false, "", err
	}
	if months <= 0 {
		return false, "", fmt.Errorf("%w: premium months must be positive", ErrFragmentClient)
	}

	username = strings.TrimLeft(strings.TrimSpace(username), "@")
	if username == "" {
		return false, "", nil
	}

	data := map[string]string{
		"query":  username,
		"months": strconv.Itoa(months),
		"method": "searchPremiumGiftRecipient",
	}

	client.mu.Lock()
	hash := client.hashV
	client.mu.Unlock()
	if hash == "" {
		if err := client.RefreshHash(ctx); err != nil {
			return false, "", err
		}
	}

	payload, err := client.postFragment(ctx, data)
	if err != nil {
		return false, "", err
	}

	if errText := anyToString(payload["error"]); errText != "" {
		if errText == "Bad request" || errText == "Unknown error" {
			if err := client.RefreshHash(ctx); err != nil {
				return false, "", err
			}
			payload, err = client.postFragment(ctx, data)
			if err != nil {
				return false, "", err
			}
			errText = anyToString(payload["error"])
		}
		if strings.Contains(strings.ToLower(errText), "already subscribed") {
			return true, username, nil
		}
		if errText != "" {
			return false, "", fmt.Errorf("%w: %s", ErrFragmentClient, errText)
		}
	}

	found, _ := payload["found"].(map[string]any)
	if found == nil {
		return false, "", nil
	}

	return false, username, nil
}

func (client *Fragment) RefreshHash(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fragmentHomeURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cache-Control", "no-cache")
	addCookies(req, client.cookies)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: fragment home rejected cookies", ErrFragmentAuth)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: fragment home status %d", ErrFragmentClient, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}
	html := string(body)

	var newHash string
	for _, pattern := range hashPatterns {
		match := pattern.FindStringSubmatch(html)
		if len(match) > 1 {
			newHash = match[1]
			break
		}
	}
	if newHash == "" {
		return fmt.Errorf("%w: не удалось обновить hash", ErrFragmentAuth)
	}

	client.mu.Lock()
	client.hashV = newHash
	client.mu.Unlock()

	match := walletInitPattern.FindStringSubmatch(html)
	if len(match) < 2 {
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(match[1]), &data); err == nil {
		client.mu.Lock()
		defer client.mu.Unlock()

		if value, ok := data["logged_in"].(bool); ok {
			client.walletConnected = &value
		} else {
			client.walletConnected = nil
		}
	}

	return nil
}

func (client *Fragment) IsWalletConnected(ctx context.Context) error {
	client.mu.Lock()
	connected := client.walletConnected
	client.mu.Unlock()

	if connected == nil {
		if err := client.RefreshHash(ctx); err != nil {
			return err
		}

		client.mu.Lock()
		connected = client.walletConnected
		client.mu.Unlock()
	}

	if connected != nil && !*connected {
		return fmt.Errorf("%w: к Fragment не подключен wallet", ErrFragmentAuth)
	}

	return nil
}

func (client *Fragment) IsFragmentConnected() error {
	if len(client.cookies) == 0 {
		return fmt.Errorf("%w: Fragment Cookies не заданы", ErrFragmentAuth)
	}

	var missing []string
	for _, name := range requiredFragmentCookies {
		if _, ok := client.cookies[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: FRAGMENT_COOKIES не содержит: %s", ErrFragmentAuth, strings.Join(missing, ", "))
	}
	if client.seed == "" {
		return fmt.Errorf("%w: WALLET_SEED не задан", ErrFragmentAuth)
	}
	if client.tonKey == "" {
		return fmt.Errorf("%w: TON_CONSOLE не задан", ErrFragmentAuth)
	}

	return nil
}

func (client *Fragment) SendTON(ctx context.Context, destination string, amountNano string, payload string) (string, error) {
	if client.wallet == nil || client.liteClient == nil {
		if err := client.Start(ctx); err != nil {
			return "", err
		}
	}

	dstAddr, err := tonaddr.ParseAddr(destination)
	if err != nil {
		return "", fmt.Errorf("%w: invalid destination address: %v", ErrFragmentClient, err)
	}

	amount, err := tlb.FromNanoTONStr(amountNano)
	if err != nil {
		return "", fmt.Errorf("%w: invalid nano amount: %v", ErrFragmentClient, err)
	}

	payloadBOC, err := base64.StdEncoding.DecodeString(padBase64(payload))
	if err != nil {
		return "", fmt.Errorf("%w: invalid base64 payload: %v", ErrFragmentClient, err)
	}

	body, err := cell.FromBOC(payloadBOC)
	if err != nil {
		return "", fmt.Errorf("%w: invalid payload boc: %v", ErrFragmentClient, err)
	}

	stickyCtx := client.liteClient.StickyContext(ctx)
	msg := tonwallet.SimpleMessage(dstAddr, amount, body)
	txHash, err := client.wallet.SendManyWaitTxHash(stickyCtx, []*tonwallet.Message{msg})
	if err != nil {
		return "", fmt.Errorf("%w: TON transfer failed: %v", ErrFragmentClient, err)
	}

	return base64.StdEncoding.EncodeToString(txHash), nil
}

func (client *Fragment) postFragment(ctx context.Context, data map[string]string) (map[string]any, error) {
	client.mu.Lock()
	hash := client.hashV
	client.mu.Unlock()

	form := url.Values{}
	for key, value := range data {
		form.Set(key, value)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fragmentAPIURL+"?hash="+url.QueryEscape(hash),
		bytes.NewBufferString(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}

	for key, value := range fragmentHeaders {
		req.Header.Set(key, value)
	}
	addCookies(req, client.cookies)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFragmentTimeout, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: fragment status %d", ErrFragmentClient, resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode fragment response: %v", ErrFragmentClient, err)
	}

	return payload, nil
}

func (client *Fragment) FragmentAPI(ctx context.Context, data map[string]string) (map[string]any, error) {
	if err := client.IsFragmentConnected(); err != nil {
		return nil, err
	}

	method := data["method"]
	if method == "initBuyStarsRequest" || method == "getBuyStarsLink" {
		if err := client.IsWalletConnected(ctx); err != nil {
			return nil, err
		}
	}

	client.mu.Lock()
	hash := client.hashV
	client.mu.Unlock()

	if hash == "" {
		if err := client.RefreshHash(ctx); err != nil {
			return nil, err
		}
	}

	payload, err := client.postFragment(ctx, data)
	if err != nil {
		return nil, err
	}

	errText := anyToString(payload["error"])
	if errText == "Bad request" || errText == "Unknown error" {
		if err := client.RefreshHash(ctx); err != nil {
			return nil, err
		}

		payload, err = client.postFragment(ctx, data)
		if err != nil {
			return nil, err
		}
	}

	if err := client.raiseError(payload, method); err != nil {
		return nil, err
	}

	return payload, nil
}

func (client *Fragment) raiseError(payload map[string]any, method string) error {
	errText := anyToString(payload["error"])
	if errText == "" {
		return nil
	}

	needVerify, _ := payload["need_verify"].(bool)
	if needVerify {
		return fmt.Errorf(
			"%w: Fragment запросил дополнительную верификацию (need_verify=true), method=%s",
			ErrFragmentVerification,
			method,
		)
	}

	if errText == "Access denied" {
		client.mu.Lock()
		connected := client.walletConnected
		client.mu.Unlock()

		if connected != nil && !*connected {
			return fmt.Errorf("%w: fragment session not connected to TON wallet", ErrFragmentAuth)
		}

		return fmt.Errorf("%w: access denied, reconnect TON in Fragment and refresh FRAGMENT_COOKIES", ErrFragmentAuth)
	}

	return fmt.Errorf("%w: %s", ErrFragmentClient, errText)
}

func extractTransactionMessage(payload map[string]any) (*fragmentTxMessage, error) {
	transaction, _ := payload["transaction"].(map[string]any)
	if transaction == nil {
		return nil, fmt.Errorf("%w: Fragment не вернул transaction", ErrFragmentClient)
	}

	messages, _ := transaction["messages"].([]any)
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: Fragment не вернул messages в transaction", ErrFragmentClient)
	}

	message, _ := messages[0].(map[string]any)
	if message == nil {
		return nil, fmt.Errorf("%w: invalid transaction message", ErrFragmentClient)
	}

	address := anyToString(message["address"])
	amount := anyToString(message["amount"])
	encodedPayload := anyToString(message["payload"])

	if address == "" {
		return nil, fmt.Errorf("%w: Fragment transaction message без поля address", ErrFragmentClient)
	}
	if amount == "" {
		return nil, fmt.Errorf("%w: Fragment transaction message без поля amount", ErrFragmentClient)
	}
	if encodedPayload == "" {
		return nil, fmt.Errorf("%w: Fragment transaction message без поля payload", ErrFragmentClient)
	}

	return &fragmentTxMessage{
		Address: address,
		Amount:  amount,
		Payload: encodedPayload,
	}, nil
}

func walletVersionConfig(raw string) (tonwallet.VersionConfig, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "W5", "V5", "V5R1", "W5R1":
		return tonwallet.ConfigV5R1Final{
			NetworkGlobalID: tonwallet.MainnetGlobalID,
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported WALLET_VERSION %q", ErrFragmentAuth, raw)
	}
}

func anyToString(value any) string {
	switch item := value.(type) {
	case nil:
		return ""
	case string:
		return item
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatInt(int64(item), 10)
	case float32:
		return strconv.FormatInt(int64(item), 10)
	case int:
		return strconv.Itoa(item)
	case int64:
		return strconv.FormatInt(item, 10)
	case uint64:
		return strconv.FormatUint(item, 10)
	default:
		return fmt.Sprint(item)
	}
}

func normalizeSeed(seed string) (string, error) {
	words := strings.Fields(seed)
	if len(words) == 0 {
		return "", nil
	}
	if len(words) != 12 && len(words) != 24 {
		return "", fmt.Errorf("%w: WALLET_SEED должен содержать 12 или 24 слова", ErrFragmentAuth)
	}
	return strings.Join(words, " "), nil
}

func parseCookies(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}

	out := make(map[string]string)
	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key != "" {
			out[key] = val
		}
	}

	return out
}

func padBase64(value string) string {
	if mod := len(value) % 4; mod != 0 {
		return value + strings.Repeat("=", 4-mod)
	}
	return value
}

func addCookies(req *http.Request, cookies map[string]string) {
	for key, value := range cookies {
		req.AddCookie(&http.Cookie{Name: key, Value: value})
	}
}

var FragmentService *Fragment

func InitFragment() error {
	client, err := NewFragmentClient()
	if err != nil {
		return err
	}

	FragmentService = client
	return nil
}

func BuyStarsSimple(ctx context.Context, user string, amount int) (string, error) {
	if FragmentService == nil {
		return "", fmt.Errorf("%w: fragment client is nil", ErrFragmentClient)
	}

	if err := FragmentService.Start(ctx); err != nil {
		return "", err
	}

	result, err := FragmentService.BuyStars(ctx, user, amount)
	if err != nil {
		return "", err
	}

	return result.TxHash, nil
}
