package utils

import (
	config "adamant/app/bot/config"

	api "github.com/mymmrac/telego"
)

func CheckAdmin(tg_id int64) bool {
	if tg_id == config.Cfg.Admin {
		return true
	} else {
		return false
	}
}

func IsCommand(msg *api.Message) bool {
	if msg == nil || msg.Text == "" || len(msg.Entities) == 0 {
		return false
	}

	entity := msg.Entities[0]
	return entity.Type == api.EntityTypeBotCommand && entity.Offset == 0
}