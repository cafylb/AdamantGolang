package utils

import (
	"context"
	"log"

	api "github.com/mymmrac/telego"
)

func Answer(bot *api.Bot, chatID int64, text string, keyboard ...*api.InlineKeyboardMarkup) {
	message := api.SendMessageParams{
		ChatID:             api.ChatID{ID: chatID},
		Text:               text,
		ParseMode:          api.ModeHTML,
		LinkPreviewOptions: &api.LinkPreviewOptions{IsDisabled: true},
	}

	if len(keyboard) > 0 && keyboard[0] != nil {
		message.ReplyMarkup = keyboard[0]
	}

	_, err := bot.SendMessage(context.Background(), &message)
	if err != nil {
		log.Println(err)
	}
}

func Reply(bot *api.Bot, msg *api.Message, text string, keyboard ...*api.InlineKeyboardMarkup) {
	message := &api.SendMessageParams{
		ChatID:             api.ChatID{ID: msg.Chat.ID},
		Text:               text,
		ParseMode:          api.ModeHTML,
		LinkPreviewOptions: &api.LinkPreviewOptions{IsDisabled: true},
		ReplyParameters: (&api.ReplyParameters{
			MessageID: msg.MessageID,
		}).WithAllowSendingWithoutReply(),
	}

	if len(keyboard) > 0 && keyboard[0] != nil {
		message.ReplyMarkup = keyboard[0]
	}

	_, err := bot.SendMessage(context.Background(), message)
	if err != nil {
		log.Println(err)
	}
}

func Edit(bot *api.Bot, msg *api.Message, text string, keyboard ...*api.InlineKeyboardMarkup) {
	params := &api.EditMessageTextParams{
		ChatID:    api.ChatID{ID: msg.Chat.ID},
		MessageID: msg.MessageID,
		Text:      text,
		ParseMode: api.ModeHTML,
		LinkPreviewOptions: &api.LinkPreviewOptions{
			IsDisabled: true,
		},
	}

	if len(keyboard) > 0 && keyboard[0] != nil {
		params.ReplyMarkup = keyboard[0]
	}

	if _, err := bot.EditMessageText(context.Background(), params); err != nil {
		log.Println(err)
	}
}

func EditKeyboard(bot *api.Bot, msg *api.Message, keyboard api.InlineKeyboardMarkup) {
	params := &api.EditMessageReplyMarkupParams{
		ChatID: api.ChatID{ID: msg.Chat.ID},
		MessageID: msg.MessageID,
		ReplyMarkup: &keyboard,
	}

	if _, err := bot.EditMessageReplyMarkup(context.Background(), params); err != nil {
		log.Println(err)
	}
}

func EditById(bot *api.Bot, msg_id int64, chat_id int64, text string, keyboard ...*api.InlineKeyboardMarkup) {
	params := &api.EditMessageTextParams{
		ChatID: api.ChatID{ID: chat_id},
		MessageID: int(msg_id),
		Text: text,
		ParseMode: api.ModeHTML,
		LinkPreviewOptions: &api.LinkPreviewOptions{
			IsDisabled: true,
		},
	}

	if len(keyboard) > 0 && keyboard[0] != nil {
		params.ReplyMarkup = keyboard[0]
	}

	if _, err := bot.EditMessageText(context.Background(), params); err != nil {
		log.Println(err)
	}
}

func CallbackAnswer(bot *api.Bot, callback *api.CallbackQuery, text ...string) {
	params := &api.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	}

	if len(text) > 0 && text[0] != "" {
		params.Text = text[0]
	}

	if err := bot.AnswerCallbackQuery(context.Background(), params); err != nil {
		log.Println(err)
	}
}

func DeleteMessage(bot *api.Bot, message *api.Message) {
	params := &api.DeleteMessageParams{
		ChatID:    api.ChatID{ID: message.Chat.ID},
		MessageID: message.MessageID,
	}

	if err := bot.DeleteMessage(context.Background(), params); err != nil {
		log.Println(err)
	}
}
