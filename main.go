package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/openai"

	// "github.com/tztsai/openai-telegram/src/session"
	"github.com/tztsai/openai-telegram/src/tgbot"
)

func main() {
	envConfig, err := config.LoadEnvConfig(".env")
	if err != nil {
		log.Fatalf("Couldn't load .env config: %v", err)
	}
	if err := envConfig.ValidateWithDefaults(); err != nil {
		log.Fatalf("Invalid .env config: %v", err)
	}

	gpt := openai.Init(envConfig)
	log.Println("Started GPT-4")

	bot, err := tgbot.New(envConfig.TelegramToken, time.Duration(envConfig.EditWaitSeconds*int(time.Second)))
	if err != nil {
		log.Fatalf("Couldn't start Telegram bot: %v", err)
	}

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		bot.Stop()
		os.Exit(0)
	}()

	log.Printf("Started Telegram bot! Message @%s to start.", bot.Username)

	for update := range bot.GetUpdatesChan() {
		if update.Message == nil {
			continue
		}

		var (
			updateText      = update.Message.Text
			updateChatID    = update.Message.Chat.ID
			updateMessageID = update.Message.MessageID
			updateUserID    = update.Message.From.ID
		)

		if len(envConfig.TelegramID) != 0 && !envConfig.HasTelegramID(updateUserID) {
			log.Printf("User %d is not allowed to use this bot", updateUserID)
			bot.Send(updateChatID, updateMessageID, "You are not authorized to use this bot.")
			continue
		}

		if !update.Message.IsCommand() {
			log.Println("Received message:\n", updateText)

			bot.SendTyping(updateChatID)

			feed, err, infos := gpt.SendMessage(updateText, updateChatID)
			for _, info := range infos {
				bot.Send(updateChatID, updateMessageID, "INFO: "+info)
			}
			if err != nil {
				bot.Send(updateChatID, updateMessageID, fmt.Sprintf("ERROR: %v", err))
			} else if feed != nil {
				bot.SendAsLiveOutput(updateChatID, updateMessageID, feed)
			}
			continue
		}

		var text string
		cmd := update.Message.Command()
		switch cmd {
		case "help":
			text = "Send a message to start talking with GPT4. You can use /reset at any point to clear the conversation history and start from scratch (don't worry, it won't delete the Telegram messages)."
		case "start":
			text = "Send a message to start talking with GPT4. You can use /reset at any point to clear the conversation history and start from scratch (don't worry, it won't delete the Telegram messages)."
		case "reset":
			gpt.ResetConversation(updateChatID)
			text = "Started a new conversation. Enjoy!"
		case "chats":
			for _, chatID := range gpt.GetChats() {
				text += fmt.Sprintf("/Chat%d\n", chatID)
			}
		default:
			if strings.HasPrefix(cmd, "Chat") {
				i, err := strconv.Atoi(cmd[4:])
				if err != nil {
					text = "Unknown chat ID."
				} else {
					ms := gpt.Conversations[int64(i)].Messages
					bs, _ := json.Marshal(&ms)
					text = string(bs)
				}
			} else {
				text = "Unknown command. Send /help to see a list of commands."
			}
		}

		if _, err := bot.Send(updateChatID, updateMessageID, text); err != nil {
			log.Printf("Error sending message: %v", err)
		}
	}
}
