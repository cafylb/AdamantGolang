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
