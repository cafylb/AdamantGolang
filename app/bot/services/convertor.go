package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"
)

var (
	ErrConvertorClient      = errors.New("convertor client error")
	ErrConvertorUnavailable = errors.New("convertor rates unavailable")
)

const (
	convertorUpdateInterval = 5 * time.Minute
	convertorHTTPTimeout    = 10 * time.Second
	coinGeckoSimplePriceURL = "https://api.coingecko.com/api/v3/simple/price?ids=the-open-network&vs_currencies=usd,rub&include_last_updated_at=true&precision=full"
)

type Rates struct {
	TONUSD    float64
	USDRUB    float64
	UpdatedAt time.Time
	Source    string
}

type Convertor struct {
	mu sync.RWMutex

	httpClient *http.Client
	interval   time.Duration

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
	rates   Rates
}

type coinGeckoRatesResponse struct {
	TheOpenNetwork struct {
		USD           float64 `json:"usd"`
		RUB           float64 `json:"rub"`
		LastUpdatedAt int64   `json:"last_updated_at"`
	} `json:"the-open-network"`
}

func NewConvertor() *Convertor {
	return &Convertor{
		httpClient: &http.Client{Timeout: convertorHTTPTimeout},
		interval:   convertorUpdateInterval,
	}
}

func (c *Convertor) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: convertorHTTPTimeout}
	}
	c.mu.Unlock()

	if err := c.Refresh(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	c.cancel = cancel
	c.done = done
	c.running = true

	go c.loop(runCtx, done)
	return nil
}

func (c *Convertor) Stop() error {
	c.mu.Lock()
	cancel := c.cancel
	done := c.done
	c.cancel = nil
	c.done = nil
	c.running = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	return nil
}

func (c *Convertor) loop(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, convertorHTTPTimeout)
			if err := c.Refresh(refreshCtx); err != nil {
				log.Printf("convertor refresh failed: %v", err)
			}
			cancel()
		}
	}
}

func (c *Convertor) Refresh(ctx context.Context) error {
	rates, err := c.fetchCoinGeckoRates(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.rates = rates
	c.mu.Unlock()

	log.Printf(
		"convertor rates updated: 1 TON = %.6f USD | 1 USD = %.4f RUB",
		rates.TONUSD,
		rates.USDRUB,
	)

	return nil
}

func (c *Convertor) Rates() (Rates, error) {
	c.mu.RLock()
	rates := c.rates
	c.mu.RUnlock()

	if rates.TONUSD <= 0 || rates.USDRUB <= 0 {
		return Rates{}, ErrConvertorUnavailable
	}

	return rates, nil
}

func (c *Convertor) EnsureFresh(ctx context.Context, maxAge time.Duration) (Rates, error) {
	rates, err := c.Rates()
	if err == nil && (maxAge <= 0 || time.Since(rates.UpdatedAt) <= maxAge) {
		return rates, nil
	}

	if refreshErr := c.Refresh(ctx); refreshErr != nil {
		if err == nil {
			return rates, nil
		}
		return Rates{}, refreshErr
	}

	return c.Rates()
}

func (c *Convertor) USDToTON(ctx context.Context, usd float64) (float64, error) {
	if usd < 0 {
		return 0, fmt.Errorf("%w: usd amount must be positive", ErrConvertorClient)
	}

	rates, err := c.EnsureFresh(ctx, c.interval*2)
	if err != nil {
		return 0, err
	}

	return roundFloat(usd/rates.TONUSD, 9), nil
}

func (c *Convertor) USDToRUB(ctx context.Context, usd float64) (float64, error) {
	if usd < 0 {
		return 0, fmt.Errorf("%w: usd amount must be positive", ErrConvertorClient)
	}

	rates, err := c.EnsureFresh(ctx, c.interval*2)
	if err != nil {
		return 0, err
	}

	return roundFloat(usd*rates.USDRUB, 4), nil
}

func (c *Convertor) TONToUSD(ctx context.Context, ton float64) (float64, error) {
	if ton < 0 {
		return 0, fmt.Errorf("%w: ton amount must be positive", ErrConvertorClient)
	}

	rates, err := c.EnsureFresh(ctx, c.interval*2)
	if err != nil {
		return 0, err
	}

	return roundFloat(ton*rates.TONUSD, 6), nil
}

func (c *Convertor) fetchCoinGeckoRates(ctx context.Context) (Rates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coinGeckoSimplePriceURL, nil)
	if err != nil {
		return Rates{}, fmt.Errorf("%w: build request: %v", ErrConvertorClient, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Rates{}, fmt.Errorf("%w: %v", ErrConvertorClient, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Rates{}, fmt.Errorf("%w: CoinGecko status %d", ErrConvertorClient, resp.StatusCode)
	}

	var payload coinGeckoRatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Rates{}, fmt.Errorf("%w: decode CoinGecko response: %v", ErrConvertorClient, err)
	}

	tonUSD := payload.TheOpenNetwork.USD
	tonRUB := payload.TheOpenNetwork.RUB
	if tonUSD <= 0 || tonRUB <= 0 {
		return Rates{}, fmt.Errorf("%w: invalid CoinGecko rates", ErrConvertorClient)
	}

	updatedAt := time.Now().UTC()
	if payload.TheOpenNetwork.LastUpdatedAt > 0 {
		updatedAt = time.Unix(payload.TheOpenNetwork.LastUpdatedAt, 0).UTC()
	}

	return Rates{
		TONUSD:    tonUSD,
		USDRUB:    tonRUB / tonUSD,
		UpdatedAt: updatedAt,
		Source:    "coingecko",
	}, nil
}

func roundFloat(value float64, digits int) float64 {
	if digits < 0 {
		return value
	}

	pow := math.Pow(10, float64(digits))
	return math.Round(value*pow) / pow
}

var ConvertorService *Convertor

func InitConvertor() error {
	ConvertorService = NewConvertor()
	return nil
}

func ConvertUSDToTON(ctx context.Context, usd float64) (float64, error) {
	if ConvertorService == nil {
		if err := InitConvertor(); err != nil {
			return 0, err
		}
	}

	return ConvertorService.USDToTON(ctx, usd)
}

func ConvertUSDToRUB(ctx context.Context, usd float64) (float64, error) {
	if ConvertorService == nil {
		if err := InitConvertor(); err != nil {
			return 0, err
		}
	}

	return ConvertorService.USDToRUB(ctx, usd)
}
