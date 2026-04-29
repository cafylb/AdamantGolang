package repository

import (
	"context"

	database "adamant/app/bot/database"
)

type User struct {
	TgId       int64
	StarsSpend int32
	Balance    int64
}

func CreateUser(ctx context.Context, tgID int64) error {
	_, err := database.Pool.Exec(ctx, `INSERT INTO users (tg_id) VALUES ($1) ON CONFLICT (tg_id) DO NOTHING`, tgID)

	return err
}

func GetUserPurchases(ctx context.Context, tgID int64) ([]Purchase, error) {
	rows, err := database.Pool.Query(ctx, `SELECT tg_id, data, created_at FROM purchases WHERE tg_id = $1 AND complete = TRUE`, tgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Purchase
	var purchase Purchase

	for rows.Next() {
		err := rows.Scan(&purchase.UserID, &purchase.Data, &purchase.CreatedAt)
		if err != nil {
			return nil, err
		}
		res = append(res, purchase)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

func GetUserBalance(ctx context.Context, tgID int64) (int64, error) {
	var balance int64
	err := database.Pool.QueryRow(ctx, `SELECT balance FROM users WHERE tg_id = $1`, tgID).Scan(&balance)
	return balance, err
}

func ChangeAdamantBalance(ctx context.Context, tgID int64, amount int64) (int64, error) {
	var balance int64
	err := database.Pool.QueryRow(ctx, `UPADTE users SET balance = balance + $2 WHERE tg_id = $1 RETURNING balance`, tgID, amount).Scan(&balance)
	return balance, err
}