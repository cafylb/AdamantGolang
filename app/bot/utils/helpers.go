package utils

import (
	api "github.com/mymmrac/telego"
)

func Commands(msg *api.Message) string {
	entity := msg.Entities[0]
	cmd := (msg.Text[:entity.Length])[1:]
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == '@' {
			return cmd[:i]
		}
	}
	return cmd
}