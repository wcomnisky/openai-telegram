package tgbot

import (
	"log"
	"math"
	"os"
	"strings"
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
	// text = strings.Replace(text, "{", "\\{", -1)
	// text = strings.Replace(text, "}", "\\}", -1)
	lines := strings.Split(text, "\n")
	maxlen := 4096 // max length of a message
	block_closed := true
	content := ""
	var msg tgbotapi.Message

	for i := 0; i <= len(lines); i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		} else {
			line = ""
		}

		if len(line) > maxlen {
			line = line[:maxlen-3] + "..."
			log.Println("Truncated line")
		}

		if i == len(lines) || len(content)+len(line) > maxlen {
			content, block_closed = markdown.EnsureFormatting(content, block_closed)
			c := tgbotapi.NewMessage(chatID, content)
			// c.ParseMode = "Markdown"
			c.ReplyToMessageID = replyTo
			msg, err := b.api.Send(c)
			content = line
			if err != nil {
				return msg, err
			}
		} else {
			content += "\n" + line
		}
	}
	return msg, nil
}

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

		message, err := b.Send(chatID, replyTo, queue[0])

		if err != nil {
			log.Fatalf("Couldn't send message: %v", err)
		} else {
			l := math.Min(30, float64(len(queue[0])))
			log.Printf("Sent message: %s...", queue[0][:int(l)])
			queue = queue[1:]
			lastEditTime = time.Now()
			replyTo = message.MessageID
		}
	}
}

func (b *Bot) SendPhoto(chatID int64, photoPath string) {
	path := tgbotapi.FilePath(photoPath)
	photo := tgbotapi.NewPhoto(chatID, path)
	if _, err := b.api.Send(photo); err != nil {
		log.Printf("Couldn't send photo: %v", err)
	}
}
