package tgbot

import (
	"log"
	"os"
	"regexp"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tztsai/openai-telegram/src/markdown"
)

type Bot struct {
	Username     string
	api          *tgbotapi.BotAPI
	editInterval time.Duration
}

func New(token string, editInterval time.Duration) (*Bot, error) {
	var api *tgbotapi.BotAPI
	var err error
	apiEndpoint, exist := os.LookupEnv("TELEGRAM_API_ENDPOINT")
	if exist && apiEndpoint != "" {
		api, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	} else {
		api, err = tgbotapi.NewBotAPI(token)
	}
	if err != nil {
		return nil, err
	}
	return &Bot{
		Username:     api.Self.UserName,
		api:          api,
		editInterval: editInterval,
	}, nil
}

func (b *Bot) GetUpdatesChan() tgbotapi.UpdatesChannel {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 30
	return b.api.GetUpdatesChan(cfg)
}

func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
}

func (b *Bot) Send(chatID int64, replyTo int, text string) (tgbotapi.Message, error) {
	text = markdown.EnsureFormatting(text)
	maxlen := 3072
	space := regexp.MustCompile(`\s`)
	i0, i1 := 0, 0
	var m tgbotapi.Message
	var err error
	for i1 < len(text) {
		i0 = i1
		if i0 > 0 {
			time.Sleep(b.editInterval * time.Second)
			k := space.FindIndex([]byte(text[i0-72 : i0]))
			if k != nil { // try to split at a space
				i0 = i0 - 72 + k[1]
			}
		}
		i1 = i0 + maxlen
		if i1 >= len(text) {
			i1 = len(text)
		} else {
			k := space.FindIndex([]byte(text[i1-72 : i1]))
			if k != nil { // try to split at a space
				i1 = i1 - 72 + k[1]
			}
		}
		msg := tgbotapi.NewMessage(chatID, text[i0:i1])
		msg.ParseMode = "Markdown"
		msg.ReplyToMessageID = replyTo
		m, err = b.api.Send(msg)
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

// func (b *Bot) SendEdit(chatID int64, messageID int, text string) error {
// 	text = markdown.EnsureFormatting(text)
// 	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
// 	msg.ParseMode = "Markdown"
// 	if _, err := b.api.Send(msg); err != nil {
// 		if err.Error() == "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message" {
// 			return nil
// 		}
// 		return err
// 	}
// 	return nil
// }

func (b *Bot) SendTyping(chatID int64) {
	if _, err := b.api.Request(tgbotapi.NewChatAction(chatID, "typing")); err != nil {
		log.Printf("Couldn't send typing action: %v", err)
	}
}

func (b *Bot) SendAsLiveOutput(chatID int64, replyTo int, feed chan string) {
	var queue []string
	var lastEditTime time.Time
	var lastTypeTime time.Time
	var done = false

	for !done || len(queue) > 0 {
		if time.Since(lastTypeTime) > 10*time.Second {
			b.SendTyping(chatID)
			lastTypeTime = time.Now()
		}

		if !done {
			select {
			case response, ok := <-feed:
				if !ok {
					done = true
				} else {
					queue = append(queue, response)
				}
			}
		}

		if len(queue) == 0 || time.Since(lastEditTime) < b.editInterval {
			continue
		}

		log.Println("Sending message to chat")
		message, err := b.Send(chatID, replyTo, queue[0])

		if err != nil {
			log.Fatalf("Couldn't send message: %v", err)
		} else {
			queue = queue[1:]
			lastEditTime = time.Now()
			replyTo = message.MessageID
		}
	}

}
