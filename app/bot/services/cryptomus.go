package services

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	config "adamant/app/bot/config"
)

var (
	ErrCryptomusClient    = errors.New("cryptomus client error")
	ErrCryptomusAuth      = errors.New("cryptomus auth error")
	ErrCryptomusWebhook   = errors.New("cryptomus webhook error")
	ErrCryptomusBadStatus = errors.New("cryptomus bad status")
)

const (
	cryptomusBaseURL   = "https://api.cryptomus.com/v1"
	cryptomusTimeout   = 15 * time.Second
	cryptomusCurrency  = "USD"
	cryptomusPayAPI    = "/payment"
	cryptomusInfoAPI   = "/payment/info"
	cryptomusSignField = "sign"
)

type CryptomusClient struct {
	merchantID string
	paymentKey string
	payoutKey  string
	webhookURL string

	httpClient *http.Client
}

type CryptomusInvoiceRequest struct {
	OrderID     string   `json:"order_id"`
	Amount      string   `json:"amount"`
	Currency    string   `json:"currency,omitempty"`
	Network     string   `json:"network,omitempty"`
	URLReturn   string   `json:"url_return,omitempty"`
	URLSuccess  string   `json:"url_success,omitempty"`
	URLCallback string   `json:"url_callback,omitempty"`
	Currencies  []string `json:"currencies,omitempty"`
	Lifetime    int      `json:"lifetime,omitempty"`
	ToCurrency  string   `json:"to_currency,omitempty"`
}

type CryptomusInvoice struct {
	UUID      string `json:"uuid"`
	OrderID   string `json:"order_id"`
	URL       string `json:"url"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	Status    string `json:"status"`
	ExpiredAt string `json:"expired_at"`
}

type CryptomusWebhook struct {
	Event          string `json:"type"`
	UUID           string `json:"uuid"`
	OrderID        string `json:"order_id"`
	MerchantOrder  string `json:"merchant_order_id"`
	Status         string `json:"status"`
	Currency       string `json:"currency"`
	Amount         string `json:"amount"`
	Network        string `json:"network"`
	PaymentAmount  string `json:"payer_amount"`
	PaymentCurrency string `json:"payer_currency"`
	Raw            json.RawMessage `json:"-"`
}

type cryptomusResponse[T any] struct {
	State   any    `json:"state"`
	Message string `json:"message"`
	Result  T      `json:"result"`
}

func NewCryptomusClient() (*CryptomusClient, error) {
	client := &CryptomusClient{
		merchantID: strings.TrimSpace(config.Cfg.CryptomusMerchantId),
		paymentKey: strings.TrimSpace(config.Cfg.CryptomusPayment),
		payoutKey:  strings.TrimSpace(config.Cfg.CryptomusPayout),
		webhookURL: strings.TrimSpace(config.Cfg.CryptomusWebhookUrl),
		httpClient: &http.Client{Timeout: cryptomusTimeout},
	}

	if err := client.Validate(); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *CryptomusClient) Validate() error {
	if c.merchantID == "" {
		return fmt.Errorf("%w: CRYPTOMUS_MERCHANT_ID is empty", ErrCryptomusAuth)
	}
	if c.paymentKey == "" {
		return fmt.Errorf("%w: CRYPTOMUS_MERCHANT_PAYMENT is empty", ErrCryptomusAuth)
	}
	return nil
}

func (c *CryptomusClient) CreateInvoice(ctx context.Context, request CryptomusInvoiceRequest) (*CryptomusInvoice, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	request.OrderID = strings.TrimSpace(request.OrderID)
	request.Amount = strings.TrimSpace(request.Amount)
	if request.OrderID == "" {
		return nil, fmt.Errorf("%w: order_id is empty", ErrCryptomusClient)
	}
	if request.Amount == "" {
		return nil, fmt.Errorf("%w: amount is empty", ErrCryptomusClient)
	}
	if request.Currency == "" {
		request.Currency = cryptomusCurrency
	}
	if request.URLCallback == "" {
		request.URLCallback = c.webhookURL
	}

	var response cryptomusResponse[CryptomusInvoice]
	if err := c.post(ctx, cryptomusPayAPI, request, &response); err != nil {
		return nil, err
	}

	if strings.TrimSpace(response.Result.UUID) == "" || strings.TrimSpace(response.Result.URL) == "" {
		return nil, fmt.Errorf("%w: invoice response is incomplete", ErrCryptomusBadStatus)
	}

	return &response.Result, nil
}

func (c *CryptomusClient) GetInvoice(ctx context.Context, orderID string) (*CryptomusInvoice, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, fmt.Errorf("%w: order_id is empty", ErrCryptomusClient)
	}

	payload := map[string]string{
		"order_id": orderID,
	}

	var response cryptomusResponse[CryptomusInvoice]
	if err := c.post(ctx, cryptomusInfoAPI, payload, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *CryptomusClient) ParseWebhook(r *http.Request) (*CryptomusWebhook, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: request is nil", ErrCryptomusWebhook)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read webhook body: %v", ErrCryptomusWebhook, err)
	}

	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	if err := c.VerifyWebhook(body); err != nil {
		return nil, err
	}

	var webhook CryptomusWebhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		return nil, fmt.Errorf("%w: decode webhook payload: %v", ErrCryptomusWebhook, err)
	}
	webhook.Raw = append(webhook.Raw[:0], body...)

	if webhook.OrderID == "" {
		webhook.OrderID = webhook.MerchantOrder
	}
	if strings.TrimSpace(webhook.OrderID) == "" {
		return nil, fmt.Errorf("%w: webhook does not contain order id", ErrCryptomusWebhook)
	}

	return &webhook, nil
}

func (c *CryptomusClient) VerifyWebhook(body []byte) error {
	if len(body) == 0 {
		return fmt.Errorf("%w: empty webhook body", ErrCryptomusWebhook)
	}

	sign, compactBody, err := extractCryptomusSign(body)
	if err != nil {
		return err
	}

	if sign == "" {
		return fmt.Errorf("%w: webhook sign is empty", ErrCryptomusWebhook)
	}

	expected := cryptomusSign(compactBody, c.paymentKey)
	if strings.EqualFold(sign, expected) {
		return nil
	}

	escaped := bytes.ReplaceAll(compactBody, []byte(`/`), []byte(`\/`))
	if strings.EqualFold(sign, cryptomusSign(escaped, c.paymentKey)) {
		return nil
	}

	return fmt.Errorf("%w: invalid webhook sign", ErrCryptomusWebhook)
}

func (c *CryptomusClient) WebhookHandler(process func(context.Context, *CryptomusWebhook) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		webhook, err := c.ParseWebhook(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if process != nil {
			if err := process(r.Context(), webhook); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func (c *CryptomusClient) post(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrCryptomusClient, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cryptomusBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrCryptomusClient, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("merchant", c.merchantID)
	req.Header.Set("sign", cryptomusSign(body, c.paymentKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCryptomusClient, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrCryptomusClient, err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: cryptomus rejected merchant credentials", ErrCryptomusAuth)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: status %d: %s", ErrCryptomusBadStatus, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrCryptomusClient, err)
	}

	return nil
}

func extractCryptomusSign(body []byte) (string, []byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, fmt.Errorf("%w: decode webhook body: %v", ErrCryptomusWebhook, err)
	}

	signRaw, ok := payload[cryptomusSignField]
	if !ok {
		return "", nil, fmt.Errorf("%w: webhook sign field is missing", ErrCryptomusWebhook)
	}

	var sign string
	if err := json.Unmarshal(signRaw, &sign); err != nil {
		return "", nil, fmt.Errorf("%w: decode webhook sign: %v", ErrCryptomusWebhook, err)
	}

	withoutSign, err := stripTopLevelJSONField(body, cryptomusSignField)
	if err != nil {
		return "", nil, err
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, withoutSign); err != nil {
		return "", nil, fmt.Errorf("%w: compact webhook body: %v", ErrCryptomusWebhook, err)
	}

	return sign, compact.Bytes(), nil
}

func stripTopLevelJSONField(body []byte, field string) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("%w: webhook body must be a json object", ErrCryptomusWebhook)
	}

	var result bytes.Buffer
	result.Grow(len(trimmed))
	result.WriteByte('{')

	i := 1
	first := true
	for i < len(trimmed) {
		i = skipJSONSpace(trimmed, i)
		if i >= len(trimmed) {
			break
		}
		if trimmed[i] == '}' {
			result.WriteByte('}')
			return result.Bytes(), nil
		}

		keyStart := i
		keyEnd, err := consumeJSONString(trimmed, keyStart)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCryptomusWebhook, err)
		}

		var key string
		if err := json.Unmarshal(trimmed[keyStart:keyEnd], &key); err != nil {
			return nil, fmt.Errorf("%w: decode json key: %v", ErrCryptomusWebhook, err)
		}

		i = skipJSONSpace(trimmed, keyEnd)
		if i >= len(trimmed) || trimmed[i] != ':' {
			return nil, fmt.Errorf("%w: malformed webhook body", ErrCryptomusWebhook)
		}
		i++

		valueStart := skipJSONSpace(trimmed, i)
		valueEnd, err := consumeJSONValue(trimmed, valueStart)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCryptomusWebhook, err)
		}

		if key != field {
			if !first {
				result.WriteByte(',')
			}
			first = false
			result.Write(trimmed[keyStart:keyEnd])
			result.WriteByte(':')
			result.Write(trimmed[valueStart:valueEnd])
		}

		i = skipJSONSpace(trimmed, valueEnd)
		if i < len(trimmed) && trimmed[i] == ',' {
			i++
			continue
		}
		if i < len(trimmed) && trimmed[i] == '}' {
			result.WriteByte('}')
			return result.Bytes(), nil
		}
	}

	return nil, fmt.Errorf("%w: malformed webhook body", ErrCryptomusWebhook)
}

func consumeJSONValue(body []byte, start int) (int, error) {
	if start >= len(body) {
		return 0, io.ErrUnexpectedEOF
	}

	switch body[start] {
	case '"':
		return consumeJSONString(body, start)
	case '{':
		return consumeJSONComposite(body, start, '{', '}')
	case '[':
		return consumeJSONComposite(body, start, '[', ']')
	default:
		i := start
		for i < len(body) {
			switch body[i] {
			case ',', '}', ']', ' ', '\n', '\r', '\t':
				return i, nil
			}
			i++
		}
		return i, nil
	}
}

func consumeJSONComposite(body []byte, start int, open, close byte) (int, error) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(body); i++ {
		ch := body[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}

	return 0, io.ErrUnexpectedEOF
}

func consumeJSONString(body []byte, start int) (int, error) {
	if start >= len(body) || body[start] != '"' {
		return 0, fmt.Errorf("expected string at position %d", start)
	}

	escaped := false
	for i := start + 1; i < len(body); i++ {
		ch := body[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			return i + 1, nil
		}
	}

	return 0, io.ErrUnexpectedEOF
}

func skipJSONSpace(body []byte, index int) int {
	for index < len(body) {
		switch body[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

func cryptomusSign(body []byte, key string) string {
	sum := md5.Sum([]byte(base64.StdEncoding.EncodeToString(body) + key))
	return hex.EncodeToString(sum[:])
}

var CryptomusService *CryptomusClient

func InitCryptomus() error {
	client, err := NewCryptomusClient()
	if err != nil {
		return err
	}

	CryptomusService = client
	return nil
}
