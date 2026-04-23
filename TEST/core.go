package test

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Core struct {
	ID                int32
	StarsBought       int32
	CameByReferal     int32
	CompletedOrders   int32
	ViaAdamantStars   int32
	ViaTonStars       int32
	ViaCryptomusStars int32
}

func EnsureCore(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO core (id) VALUES (1)
		ON CONFLICT (id) DO NOTHING
	`)
	return err
}

/*
смотри основываясь на том рисунке
проверь все [gifts.go](app/bot/services/gifts.go) , [fragment.go](app/bot/services/fragment.go) , [cryptomus.go](app/bot/services/cryptomus.go) на работоспособность и 100% надеждность
можешь исправить все сразу, но не меняй код снаружи файлов
по ним давай отчет мне
также напиши [ton.go](TEST/ton.go) , [convertor.go](app/bot/services/convertor.go) также чтобы полностью работали

смысл convertor.go:
курс и фукнция коверта долларов в TON
также конвертация долларов в рубли

их смысл такой
я еще сделал [core.go](TEST/core.go) 
cryptomus должен работать на вебхуке помни это
а вот [ton.go](app/bot/services/ton.go) и [convertor.go](app/bot/services/convertor.go)  должны работать в циклах
каждые 10 секунд ton.go проверяет транзакции, и каждые 5 минут обновляет глобальный курс в боте [convertor.go](app/bot/services/convertor.go) 
в старте бота они оба работают одновременно
оба [ton.go](app/bot/services/ton.go) и [convertor.go](app/bot/services/convertor.go)  управляются с помощью [core.go](app/bot/core/core.go)
*/

func GetCore(ctx context.Context, db *pgxpool.Pool) (Core, error) {
	var core Core

	err := db.QueryRow(ctx, `
		SELECT
			id,
			stars_bought,
			came_by_referal,
			completed_orders,
			via_adamant_stars,
			via_ton_stars,
			via_cryptomus_stars
		FROM core
		WHERE id = 1
	`).Scan(
		&core.ID,
		&core.StarsBought,
		&core.CameByReferal,
		&core.CompletedOrders,
		&core.ViaAdamantStars,
		&core.ViaTonStars,
		&core.ViaCryptomusStars,
	)
	if err != nil {
		return Core{}, err
	}

	return core, nil
}

func AddCompletedOrders(ctx context.Context, db *pgxpool.Pool, delta int32) error {
	tag, err := db.Exec(ctx, `
		UPDATE core
		SET completed_orders = completed_orders + $1
		WHERE id = 1
	`, delta)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
