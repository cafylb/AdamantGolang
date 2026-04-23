package core

import (
	"context"
	"fmt"
	"sync"

	database "adamant/app/bot/database"
	services "adamant/app/bot/services"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/mymmrac/telego"
)

type Runtime struct {
	mu sync.Mutex

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

var BotRuntime = &Runtime{}

func (r *Runtime) Start(ctx context.Context, db *pgxpool.Pool, bot *api.Bot) error {
	if db == nil {
		return fmt.Errorf("database pool is nil")
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if services.ConvertorService == nil {
		if err := services.InitConvertor(); err != nil {
			return err
		}
	}
	if services.TONService == nil {
		if err := services.InitTON(db); err != nil {
			return err
		}
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	if err := services.ConvertorService.Start(ctx); err != nil {
		cancel()
		return err
	}

	r.mu.Lock()
	r.cancel = cancel
	r.done = done
	r.running = true
	r.mu.Unlock()

	go func() {
		defer close(done)
		services.TONService.StartWorkers(runCtx, bot)
		<-runCtx.Done()
	}()

	return nil
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.running = false
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	if services.ConvertorService != nil {
		_ = services.ConvertorService.Stop()
	}
	if services.TONService != nil {
		services.TONService.Close()
	}
	if services.GiftService != nil {
		_ = services.GiftService.Stop()
	}

	return nil
}

func StartBackground(ctx context.Context, bot *api.Bot) error {
	return BotRuntime.Start(ctx, database.Pool, bot)
}

func StopBackground() error {
	return BotRuntime.Stop()
}
