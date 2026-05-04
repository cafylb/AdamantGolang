//go:build webhook
// +build webhook
// cloudflared tunnel --url http://localhost:3400
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/mymmrac/telego"

	config "adamant/app/bot/config"
	core "adamant/app/bot/core"
	i18n "adamant/app/bot/core/i18n"
	"adamant/app/bot/data/fsm"
	database "adamant/app/bot/database"
	admin "adamant/app/bot/handlers/admin"
	user "adamant/app/bot/handlers/user"
	utils "adamant/app/bot/utils"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	i18n.MustInit()

	bot, err := telego.NewBot(config.Cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewPostgres(ctx, config.Cfg.DB)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := core.StartBackground(ctx, bot); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := core.StopBackground(); err != nil {
			log.Printf("background shutdown error: %v", err)
		}
	}()

	webhookURL := config.Cfg.WebhookURL
	if webhookURL == "" {
		log.Fatal("WEBHOOK_URL not set")
	}

	if err := ensureWebhook(ctx, bot, webhookURL); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook(bot))
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/user/balance", apiUserBalance())
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			apiCreateOrder()(w, r)
		} else {
			http.Error(w, "Method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/orders/", apiGetOrder())

	server := &http.Server{
		Addr:         ":3400",
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	server.Shutdown(context.Background())
}

func ensureWebhook(ctx context.Context, bot *telego.Bot, webhookURL string) error {
	info, err := bot.GetWebhookInfo(ctx)
	if err != nil {
		return err
	}

	if info.URL == webhookURL {
		return nil
	}

	err = bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL: webhookURL,
	})

	return err
}

func handleWebhook(bot *telego.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad request", 400)
			return
		}

		var update telego.Update
		if err := json.Unmarshal(body, &update); err != nil {
			http.Error(w, "Bad request", 400)
			return
		}

		go processUpdate(bot, &update)
		w.Write([]byte("OK"))
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"ok": true}`))
}

func apiUserBalance() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		telegramID := r.URL.Query().Get("telegram_id")
		if telegramID == "" {
			http.Error(w, "telegram_id required", 400)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"telegram_id":   telegramID,
			"balance_usd":   0,
			"balance_stars": 0,
		})
	}
}

func apiCreateOrder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		var req struct {
			TelegramID int64  `json:"telegram_id"`
			Username   string `json:"username"`
			Amount     int    `json:"amount"`
			Method     string `json:"method"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}

		// TODO: Создать заказ в БД
		// orderID, err := db.CreateOrder(ctx, req)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"order_id": "temp_order_id",
			"status":   "pending",
		})
	}
}

func apiGetOrder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		orderID := r.URL.Path[len("/api/orders/"):]
		if orderID == "" {
			http.Error(w, "order_id required", 400)
			return
		}

		// TODO: Получить заказ из БД
		// order, err := db.GetOrder(ctx, orderID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"order_id": orderID,
			"status":   "pending",
			"amount":   0,
		})
	}
}

func processUpdate(bot *telego.Bot, update *telego.Update) {
	if update.CallbackQuery != nil {
		go user.HandleCallback(bot, update.CallbackQuery)
		if utils.CheckAdmin(update.CallbackQuery.From.ID) {
			go admin.HandleCallback(bot, update.CallbackQuery)
			return
		}
	}

	if update.Message == nil {
		return
	}

	if utils.IsCommand(update.Message) {
		go user.HandleCommand(bot, update.Message)
		if utils.CheckAdmin(update.Message.From.ID) {
			go admin.HandleCommand(bot, update.Message)
			return
		}
	}

	if cur := fsm.UserFSM.Has(update.Message.From.ID); cur != "" {
		user.HandleFSM(bot, update.Message, cur)
	}
}
