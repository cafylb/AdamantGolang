package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Purchase struct {
	UserID int64
	Data string
	Complete bool
	CreatedAt time.Time
}

func CreatePurchase(ctx context.Context, db *pgxpool.Pool, userID int64, data string) error {
	if exists, err := IsPurchase(ctx, db, userID, data); err != nil && exists {
		return nil
	}
	_, err := db.Exec(ctx, `INSERT INTO purchases (tg_id, data) VALUES ($1, $2)`, userID, data)

	return err
}

func IsPurchase(ctx context.Context, db *pgxpool.Pool, userID int64, data string) (bool, error) {
	exists := false
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM purchases WHERE tg_id = $1 AND data = $2)`, userID, data).Scan(&exists)

	return exists, err
}

func CompletePurchase(ctx context.Context, db *pgxpool.Pool, userID int64, data string) error {
	tag, err := db.Exec(ctx, `UPDATE purchases SET status = TRUE WHERE tg_id = $1 AND data = $2`, userID, data)
	if err != nil {
		return err
	} 
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func CleanUpPurchases(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `DELETE FROM purchases WHERE created_at <= NOW() - INTERVAL '30 minutes'`)

	return err
}