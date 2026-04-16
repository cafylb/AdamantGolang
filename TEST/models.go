package test

import (
	"time"
)

type User struct {
	TgId int64 // primary
	StarsSpend int32
	Balance string
}

/*
status types:
pending -> false
complete -> true


*/

type Purchase struct {
	User User
	Data string
	Complete bool
	CreatedAt time.Time
}