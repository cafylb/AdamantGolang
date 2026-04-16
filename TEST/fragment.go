package test

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
	"referer":         
	"https://fragment.com/",
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

func (c *Fragment) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	if c.liteClient == nil || c.tonClient == nil || c.wallet == nil {
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

		versionCfg, err := walletVersionConfig(c.walletV)
		if err != nil {
			pool.Stop()
			return err
		}	

		w, err := tonwallet.FromSeedWithOptions(api, strings.Fields(c.seed), versionCfg)
		if err != nil {
			pool.Stop()
			return fmt.Errorf("%w: failed to init TON wallet from seed: %v", ErrFragmentAuth, err)
		}

		if c.walletA != "" {
			cfgAddr, err := tonaddr.ParseAddr(c.walletA)
			if err != nil {
				pool.Stop()
				return fmt.Errorf("%w: invalid BANK address: %v", ErrFragmentAuth, err)
			}
			if !cfgAddr.Equals(w.WalletAddress()) {
				pool.Stop()
				return fmt.Errorf(
					"%w: BANK address %s does not match wallet address from seed %s",
					ErrFragmentAuth,
					cfgAddr.StringRaw(),
					w.WalletAddress().StringRaw(),
				)
			}
		}

		c.liteClient = pool
		c.tonClient = api
		c.wallet = w
	}

	return nil
}

func (c *Fragment) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.liteClient != nil {
		c.liteClient.Stop()
		c.liteClient = nil
	}

	c.tonClient = nil
	c.wallet = nil
	return nil
}

func (c *Fragment) GetWalletBalance(ctx context.Context) (float64, error) {
	if c.walletA == "" {
		return 0, fmt.Errorf("%w: BANK не задан", ErrFragmentAuth)
	}
	if c.tonKey == "" {
		return 0, fmt.Errorf("%w: TON API KEY не задан", ErrFragmentAuth)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(tonAccountURL, c.walletA), nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.tonKey)
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
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

func (c *Fragment) BuyStars(ctx context.Context, user string, amount int) (*FragmentPurchaseResult, error) {
	if err := c.Start(ctx); err != nil {
		return nil, err
	}	

	initPayload, err := c.fragmentAPI(ctx, map[string]string{
		"recipient": user,
		"quantity":  strconv.Itoa(amount),
		"method":    "initBuyStarsRequest",
	})
	if err != nil {
		return nil, err
	}

	reqID := anyToString(initPayload["req_id"])
	if reqID == "" {
		return nil, fmt.Errorf("%w: fragment не вернул req_id", ErrFragmentClient)
	}

	linkPayload, err := c.fragmentAPI(ctx, map[string]string{
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
		return nil, fmt.Errorf("%w: invalid nano amount from fragment: %v", ErrFragmentClient, err)
	}

	amountTON := float64(amountNano) / 1e9
	requiredTON := amountTON + buyFeeBufferTON

	balanceTON, err := c.GetWalletBalance(ctx)
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

	txHash, err := c.SendTON(ctx, msg.Address, msg.Amount, msg.Payload)
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

func (c *Fragment) CheckUsername(ctx context.Context, username string) (string, error) {
	payload, err := c.fragmentAPI(ctx, map[string]string{
		"query":    username,
		"method":   "searchStarsRecipient",
		"quantity": "",
	})
	if err != nil {
		return "", nil
	}

	found, _ := payload["found"].(map[string]any)
	if found == nil {
		return "", nil
	}

	recipient := anyToString(found["recipient"])
	return recipient, nil
}

func (c *Fragment) RefreshHash(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fragmentHomeURL, nil)
	if err != nil {
		return err
	}
	addCookies(req, c.cookies)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFragmentClient, err)
	}
	defer resp.Body.Close()

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

	c.mu.Lock()
	c.hashV = newHash
	c.mu.Unlock()

	match := walletInitPattern.FindStringSubmatch(html)
	if len(match) < 2 {
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(match[1]), &data); err == nil {
		if v, ok := data["logged_in"].(bool); ok {
			c.mu.Lock()
			c.walletConnected = &v
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			c.walletConnected = nil
			c.mu.Unlock()
		}
	}

	return nil
}

func (c *Fragment) IsWalletConnected(ctx context.Context) error {
	c.mu.Lock()
	connected := c.walletConnected
	c.mu.Unlock()

	if connected == nil {
		if err := c.RefreshHash(ctx); err != nil {
			return err
		}
		c.mu.Lock()
		connected = c.walletConnected
		c.mu.Unlock()
	}

	if connected != nil && !*connected {
		return fmt.Errorf("%w: к Fragment не подключен wallet", ErrFragmentAuth)
	}

	return nil
}

func (c *Fragment) IsFragmentConnected() error {
	if len(c.cookies) == 0 {
		return fmt.Errorf("%w: fragment cookies не заданы", ErrFragmentAuth)
	}

	var missing []string
	for _, name := range requiredFragmentCookies {
		if _, ok := c.cookies[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: fragment cookies не содержит: %s", ErrFragmentAuth, strings.Join(missing, ", "))
	}
	if c.seed == "" {
		return fmt.Errorf("%w: WALLET_SEED не задан", ErrFragmentAuth)
	}
	if c.tonKey == "" {
		return fmt.Errorf("%w: TON_CONSOLE не задан", ErrFragmentAuth)
	}

	return nil
}

func (c *Fragment) SendTON(ctx context.Context, destination string, amountNano string, payload string) (string, error) {
	if c.wallet == nil || c.liteClient == nil {
		if err := c.Start(ctx); err != nil {
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

	stickyCtx := c.liteClient.StickyContext(ctx)
	msg := tonwallet.SimpleMessage(dstAddr, amount, body)
	txHash, err := c.wallet.SendManyWaitTxHash(stickyCtx, []*tonwallet.Message{msg})
	if err != nil {
		return "", fmt.Errorf("%w: TON transfer failed: %v", ErrFragmentClient, err)
	}

	return base64.StdEncoding.EncodeToString(txHash), nil
}

func (c *Fragment) postFragment(ctx context.Context, data map[string]string) (map[string]any, error) {
	c.mu.Lock()
	hash := c.hashV
	c.mu.Unlock()

	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
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

	for k, v := range fragmentHeaders {
		req.Header.Set(k, v)
	}
	addCookies(req, c.cookies)

	resp, err := c.httpClient.Do(req)
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

func (c *Fragment) fragmentAPI(ctx context.Context, data map[string]string) (map[string]any, error) {
	if err := c.IsFragmentConnected(); err != nil {
		return nil, err
	}

	method := data["method"]
	if method == "initBuyStarsRequest" || method == "getBuyStarsLink" {
		if err := c.IsWalletConnected(ctx); err != nil {
			return nil, err
		}
	}

	c.mu.Lock()
	hash := c.hashV
	c.mu.Unlock()

	if hash == "" {
		if err := c.RefreshHash(ctx); err != nil {
			return nil, err
		}
	}

	payload, err := c.postFragment(ctx, data)
	if err != nil {
		return nil, err
	}

	if s := anyToString(payload["error"]); s == "Bad request" || s == "Unknown error" {
		if err := c.RefreshHash(ctx); err != nil {
			return nil, err
		}
		payload, err = c.postFragment(ctx, data)
		if err != nil {
			return nil, err
		}
	}

	if err := c.raiseError(payload, method); err != nil {
		return nil, err
	}

	return payload, nil
}					

func (c *Fragment) raiseError(payload map[string]any, method string) error {
	errText := anyToString(payload["error"])
	if errText == "" {
		return nil
	}

	needVerify, _ := payload["need_verify"].(bool)
	if needVerify {
		return fmt.Errorf(
			"%w: fragment запросил дополнительную верификацию (need_verify=true), method=%s",
			ErrFragmentVerification,
			method,
		)
	}

	if errText == "Access denied" {
		c.mu.Lock()
		connected := c.walletConnected
		c.mu.Unlock()

		if connected != nil && !*connected {
			return fmt.Errorf("%w: fragment session not connected to TON wallet", ErrFragmentAuth)
		}
		return fmt.Errorf("%w: access denied, reconnect TON in Fragment and refresh FRAGMENT_COOKIES", ErrFragmentAuth)
	}

	return fmt.Errorf("%w: %s", ErrFragmentClient, errText)
}

type fragmentTxMessage struct {
	Address string
	Amount  string
	Payload string
}

func extractTransactionMessage(payload map[string]any) (*fragmentTxMessage, error) {
	tx, _ := payload["transaction"].(map[string]any)
	if tx == nil {
		return nil, fmt.Errorf("%w: fragment не вернул transaction", ErrFragmentClient)
	}

	msgs, _ := tx["messages"].([]any)
	if len(msgs) == 0 {
		return nil, fmt.Errorf("%w: fragment не вернул messages", ErrFragmentClient)
	}

	msg, _ := msgs[0].(map[string]any)
	if msg == nil {
		return nil, fmt.Errorf("%w: invalid transaction message", ErrFragmentClient)
	}

	address := anyToString(msg["address"])
	amount := anyToString(msg["amount"])
	encodedPayload := anyToString(msg["payload"])

	if address == "" {
		return nil, fmt.Errorf("%w: fragment transaction message без поля address", ErrFragmentClient)
	}
	if amount == "" {
		return nil, fmt.Errorf("%w: fragment transaction message без поля amount", ErrFragmentClient)
	}
	if encodedPayload == "" {
		return nil, fmt.Errorf("%w: fragment transaction message без поля payload", ErrFragmentClient)
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

func anyToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case float32:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	default:
		return fmt.Sprint(x)
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
	if m := len(value) % 4; m != 0 {
		return value + strings.Repeat("=", 4-m)
	}
	return value
}

func addCookies(req *http.Request, cookies map[string]string) {
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
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
