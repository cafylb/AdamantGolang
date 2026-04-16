package services

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	config "adamant/app/bot/config"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

type Nft struct {
	Slug  string
	Num   int
	MsgID int
	Link  string
}

type GiftManager struct {
	mu sync.RWMutex

	appID       int
	appHash     string
	phone       string
	password    string
	sessionPath string

	client      *telegram.Client
	api         *tg.Client
	peerManager *peers.Manager

	cancel    context.CancelFunc
	done      chan struct{}
	startWait chan struct{}
	startErr  error
	running   bool
}

func NewGiftManager() (*GiftManager, error) {
	appID, err := strconv.Atoi(strings.TrimSpace(config.Cfg.ApiId))
	if err != nil {
		return nil, fmt.Errorf("invalid API_ID: %w", err)
	}

	return &GiftManager{
		appID:       appID,
		appHash:     strings.TrimSpace(config.Cfg.ApiHash),
		phone:       strings.TrimSpace(config.Cfg.Number),
		password:    strings.TrimSpace(os.Getenv("TG_PASSWORD")),
		sessionPath: filepath.Join(".", "manager.session.json"),
	}, nil
}

func (manager *GiftManager) Start(ctx context.Context) error {
	manager.mu.Lock()

	if manager.running {
		manager.mu.Unlock()
		return nil
	}

	if manager.startWait != nil {
		wait := manager.startWait
		manager.mu.Unlock()
		return manager.waitStart(ctx, wait)
	}

	log.Println("Gift Manager started")

	wait := make(chan struct{})
	done := make(chan struct{})
	runCtx, cancel := context.WithCancel(context.Background())

	client := telegram.NewClient(manager.appID, manager.appHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: manager.sessionPath},
	})
	api := client.API()
	peerManager := peers.Options{}.Build(api)

	manager.client = client
	manager.api = api
	manager.peerManager = peerManager
	manager.cancel = cancel
	manager.done = done
	manager.startWait = wait
	manager.startErr = nil

	manager.mu.Unlock()

	go manager.run(runCtx, client, peerManager, wait, done)
	return manager.waitStart(ctx, wait)
}

func (manager *GiftManager) Stop() error {
	manager.mu.RLock()
	cancel := manager.cancel
	done := manager.done
	manager.mu.RUnlock()

	if cancel == nil {
		return nil
	}

	cancel()
	if done != nil {
		<-done
	}

	log.Println("Manager stopped")
	return nil
}

func (manager *GiftManager) Get(ctx context.Context) ([]Nft, error) {
	result, err := manager.getSavedGifts(ctx)
	if err != nil {
		log.Printf("Ошибка в GiftManager.Get: %v", err)
		return nil, err
	}

	if len(result.Gifts) == 0 {
		log.Println("В банке нету подарков")
		return nil, nil
	}

	nfts := make([]Nft, 0, len(result.Gifts))
	sell := make([]int, 0)

	for _, gift := range result.Gifts {
		unique, ok := gift.Gift.(*tg.StarGiftUnique)
		if ok {
			nfts = append(nfts, newNft(unique.GetSlug(), unique.GetNum(), gift.MsgID))
			continue
		}

		if gift.MsgID != 0 {
			sell = append(sell, gift.MsgID)
		}
	}

	if len(sell) > 0 {
		if err := manager.Sell(ctx, sell); err != nil {
			log.Printf("Ошибка при продаже обычных подарков: %v", err)
		}
	}

	return nfts, nil
}

func (manager *GiftManager) GetWithoutSells(ctx context.Context) ([]Nft, error) {
	result, err := manager.getSavedGifts(ctx)
	if err != nil {
		log.Printf("Ошибка в GiftManager.GetWithoutSells: %v", err)
		return nil, err
	}

	if len(result.Gifts) == 0 {
		log.Println("В банке нету подарков")
		return nil, nil
	}

	nfts := make([]Nft, 0, len(result.Gifts))
	for _, gift := range result.Gifts {
		unique, ok := gift.Gift.(*tg.StarGiftUnique)
		if !ok {
			continue
		}

		nfts = append(nfts, newNft(unique.GetSlug(), unique.GetNum(), gift.MsgID))
	}

	return nfts, nil
}

func (manager *GiftManager) Sell(ctx context.Context, gifts []int) error {
	api, _, err := manager.clients(ctx)
	if err != nil {
		return err
	}

	var firstErr error

	for _, gift := range gifts {
		err := manager.sellOne(ctx, api, gift)
		if err != nil {
			log.Printf("Ошибка при продаже %d: %v", gift, err)
			if firstErr == nil {
				firstErr = err
			}
		}

		if err := waitContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}

	return firstErr
}

func (manager *GiftManager) GetRandom(ctx context.Context) (*Nft, error) {
	nfts, err := manager.Get(ctx)
	if err != nil {
		return nil, err
	}
	if len(nfts) == 0 {
		return nil, fmt.Errorf("в банке нет NFT подарков")
	}

	nft := nfts[rand.Intn(len(nfts))]
	return &nft, nil
}

func (manager *GiftManager) Transfer(ctx context.Context, nft *Nft, person int64) (bool, error) {
	if nft == nil {
		return false, fmt.Errorf("nft is nil")
	}

	api, peerManager, err := manager.clients(ctx)
	if err != nil {
		return false, err
	}

	user, err := peerManager.ResolveUserID(ctx, person)
	if err != nil {
		log.Printf("Ошибка получения пользователя %d: %v", person, err)
		return false, err
	}

	gift := &tg.InputSavedStarGiftUser{MsgID: nft.MsgID}
	peer := user.InputPeer()

	for {
		_, err = api.PaymentsTransferStarGift(ctx, &tg.PaymentsTransferStarGiftRequest{
			Stargift: gift,
			ToID:     peer,
		})
		if err == nil {
			log.Printf("Пользователю %d бесплатно отправили %s", person, nft.Link)
			return true, nil
		}

		if wait, ok := telegram.AsFloodWait(err); ok {
			if wait > 10*time.Second {
				log.Printf("При отправке Telegram сказал ждать больше 10 секунд: %s", wait)
				return false, nil
			}

			log.Printf("При отправке ждать %s", wait)
			if err := waitContext(ctx, wait+time.Second); err != nil {
				return false, err
			}

			continue
		}

		if strings.Contains(strings.ToUpper(err.Error()), "PAYMENT_REQUIRED") {
			ok, payErr := manager.transferPaid(ctx, api, gift, peer, nft, person)
			if payErr != nil {
				log.Printf("Ошибка при платной отправке %s пользователю %d: %v", nft.Link, person, payErr)
				return false, payErr
			}

			return ok, nil
		}

		log.Printf("Ошибка при отправке %s пользователю %d: %v", nft.Link, person, err)
		return false, err
	}
}

func (manager *GiftManager) TransferAll(ctx context.Context, person int64) error {
	gifts, err := manager.Get(ctx)
	if err != nil {
		return err
	}

	var firstErr error

	for i := range gifts {
		ok, err := manager.Transfer(ctx, &gifts[i], person)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if !ok && err == nil && firstErr == nil {
			firstErr = fmt.Errorf("не получилось отправить подарок %s", gifts[i].Link)
		}
	}

	return firstErr
}

func (manager *GiftManager) TranserAll(ctx context.Context, person int64) error {
	return manager.TransferAll(ctx, person)
}

func (manager *GiftManager) RandomTransfer(ctx context.Context, person int64) (bool, *Nft, error) {
	nft, err := manager.GetRandom(ctx)
	if err != nil {
		log.Printf("get_random() вернул ошибку: %v", err)
		return false, nil, err
	}

	ok, err := manager.Transfer(ctx, nft, person)
	if err != nil {
		return false, nil, err
	}
	if !ok {
		return false, nil, nil
	}

	return true, nft, nil
}

func (manager *GiftManager) run(ctx context.Context, client *telegram.Client, peerManager *peers.Manager, wait chan struct{}, done chan struct{}) {
	var once sync.Once

	markStarted := func(err error, running bool) {
		once.Do(func() {
			manager.mu.Lock()
			manager.startErr = err
			manager.running = running

			if manager.startWait == wait {
				close(manager.startWait)
				manager.startWait = nil
			}

			manager.mu.Unlock()
		})
	}

	err := client.Run(ctx, func(ctx context.Context) error {
		if err := manager.authorize(ctx, client); err != nil {
			markStarted(err, false)
			return err
		}

		if err := peerManager.Init(ctx); err != nil {
			markStarted(err, false)
			return err
		}

		me, err := client.Self(ctx)
		if err != nil {
			markStarted(err, false)
			return err
		}

		log.Printf("Успешный вход | %s | @%s", me.FirstName, strings.TrimSpace(me.Username))
		markStarted(nil, true)

		<-ctx.Done()
		return nil
	})

	markStarted(err, false)

	manager.mu.Lock()
	if manager.client == client {
		manager.running = false
		manager.client = nil
		manager.api = nil
		manager.peerManager = nil
		manager.cancel = nil
		manager.done = nil
	}
	manager.mu.Unlock()

	close(done)
}

func (manager *GiftManager) authorize(ctx context.Context, client *telegram.Client) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return err
	}
	if status.Authorized {
		return nil
	}

	codeAuth := auth.CodeAuthenticatorFunc(manager.askCode)

	if manager.password != "" {
		return auth.NewFlow(
			auth.Constant(manager.phone, manager.password, codeAuth),
			auth.SendCodeOptions{},
		).Run(ctx, client.Auth())
	}

	return auth.NewFlow(
		auth.CodeOnly(manager.phone, codeAuth),
		auth.SendCodeOptions{},
	).Run(ctx, client.Auth())
}

func (manager *GiftManager) askCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Telegram code: ")

	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(code), nil
}

func (manager *GiftManager) waitStart(ctx context.Context, wait chan struct{}) error {
	select {
	case <-wait:
		manager.mu.RLock()
		defer manager.mu.RUnlock()
		return manager.startErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *GiftManager) clients(ctx context.Context) (*tg.Client, *peers.Manager, error) {
	if err := manager.Start(ctx); err != nil {
		return nil, nil, err
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if manager.api == nil || manager.peerManager == nil {
		return nil, nil, fmt.Errorf("gift manager is not ready")
	}

	return manager.api, manager.peerManager, nil
}

func (manager *GiftManager) getSavedGifts(ctx context.Context) (*tg.PaymentsSavedStarGifts, error) {
	api, _, err := manager.clients(ctx)
	if err != nil {
		return nil, err
	}

	return api.PaymentsGetSavedStarGifts(ctx, &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:   &tg.InputPeerSelf{},
		Limit:  200,
		Offset: "",
	})
}

func (manager *GiftManager) sellOne(ctx context.Context, api *tg.Client, gift int) error {
	_, err := api.PaymentsConvertStarGift(ctx, &tg.InputSavedStarGiftUser{MsgID: gift})
	if err == nil {
		return nil
	}

	wait, ok := telegram.AsFloodWait(err)
	if !ok {
		return err
	}

	log.Printf("Надо ждать %d секунд при продаже подарка %d", int(wait/time.Second), gift)

	if err := waitContext(ctx, wait); err != nil {
		return err
	}

	_, retryErr := api.PaymentsConvertStarGift(ctx, &tg.InputSavedStarGiftUser{MsgID: gift})
	return retryErr
}

func (manager *GiftManager) transferPaid(ctx context.Context, api *tg.Client, gift *tg.InputSavedStarGiftUser, peer tg.InputPeerClass, nft *Nft, person int64) (bool, error) {
	invoice := &tg.InputInvoiceStarGiftTransfer{
		Stargift: gift,
		ToID:     peer,
	}

	form, err := api.PaymentsGetPaymentForm(ctx, &tg.PaymentsGetPaymentFormRequest{
		Invoice: invoice,
	})
	if err != nil {
		return false, err
	}

	formIDProvider, ok := form.(interface{ GetFormID() int64 })
	if !ok {
		return false, fmt.Errorf("payment form не содержит form_id")
	}

	paid, err := api.PaymentsSendStarsForm(ctx, &tg.PaymentsSendStarsFormRequest{
		FormID:  formIDProvider.GetFormID(),
		Invoice: invoice,
	})
	if err != nil {
		return false, err
	}
	if paid == nil {
		return false, fmt.Errorf("не удалось оплатить перевод подарка")
	}

	log.Printf("Платно отправлен %s пользователю %d", nft.Link, person)
	return true, nil
}

func newNft(slug string, num int, msgID int) Nft {
	return Nft{
		Slug:  slug,
		Num:   num,
		MsgID: msgID,
		Link:  fmt.Sprintf("https://t.me/nft/%s-%d", slug, num),
	}
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var GiftService *GiftManager

func InitGiftManager() error {
	manager, err := NewGiftManager()
	if err != nil {
		return err
	}

	GiftService = manager
	return nil
}