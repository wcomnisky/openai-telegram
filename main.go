package main

import (
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

	bot, err := tgbot.New(envConfig.TelegramToken,
		time.Duration(envConfig.EditWaitSeconds*int(time.Second)))
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
			conversation    = gpt.GetConversation(updateChatID)
		)

		if len(envConfig.TelegramID) != 0 && !envConfig.HasTelegramID(updateUserID) {
			log.Printf("User %d is not allowed to use this bot", updateUserID)
			bot.Send(updateChatID, updateMessageID, "You are not authorized to use this bot.")
			continue
		}

		if !update.Message.IsCommand() {
			log.Println("Received message:\n", updateText)

			bot.SendTyping(updateChatID)

			feed, err := gpt.SendMessage(updateText, updateChatID)
			// if conversation.Verbose {
			// 	for _, info := range infos {
			// 		if len(info) > 2048 {
			// 			info = info[:2048] + "..."
			// 		}
			// 		bot.Send(updateChatID, updateMessageID, "ℹ️ "+info)
			// 	}
			// }
			if err != nil {
				bot.Send(updateChatID, updateMessageID, fmt.Sprintf("❌ %v", err))
			} else if feed != nil {
				bot.SendAsLiveOutput(updateChatID, updateMessageID, feed)
			}
			continue
		}

		var text string
		cmd := update.Message.Command()
		switch cmd {
		case "help":
			text = `/reset: clear the bot's memory of this conversation.
/verbose: switch on/off the verbose mode of the bot.
/ask_friends: allow the bot to ask Bing or Wolfram Alpha before giving an answer.

A message starting with "SYSTEM:" is a system prompt.
A message starting with "TEMPER:" sets the model temperature.`
		case "start":
			text = "Send a message to start talking with GPT4. Use /help to find available commands."
		case "reset":
			gpt.ResetConversation(updateChatID)
			text = "ℹ️ Started a new conversation. Enjoy!"
		case "chats":
			for _, chatID := range gpt.GetChatIDs() {
				text += fmt.Sprintf("/chat_%d\n", chatID)
			}
		case "verbose":
			conversation.Verbose = !conversation.Verbose
			gpt.Conversations[updateChatID] = conversation
			text = fmt.Sprintf("ℹ️ verbose = %s", strconv.FormatBool(conversation.Verbose))
		case "ask_friends":
			msg := `You are allowed to send queries to your friends Bing and Wolfram Alpha before giving an answer. Whenever you are unable to answer a question or not sure about your answer, you should ask for help.

1. To send a query, a message must start with ` + "`ASK (Bing|Wolfram):\n\n`" + `. Otherwise it is regarded as your own answer.
2. Ask Bing for web searching. Bing can also be used as a calculator and unit converter for arithmetic queries, or a world clock for time-related queries.
3. Ask Wolfram for reliable data and scientific computation.
4. Ensure the accuracy of your final answer, while minimizing your number of queries and their lengths.`
			gpt.SendMessage("SYSTEM: "+msg, updateChatID)
			text = "ℹ️ Added system prompt:\n\n" + msg
		default:
			if strings.HasPrefix(cmd, "chat_") {
				i, err := strconv.Atoi(cmd[5:])
				if err != nil {
					text = "Unknown chat ID."
				} else {
					convo := gpt.Conversations[int64(i)]
					text = fmt.Sprintf("Length: %d\nTokens: %d\nStart time: %s",
						len(convo.Messages), convo.TotalTokens, convo.Time.Format(time.RFC1123Z))
					for _, msg := range convo.Messages {
						log.Println(msg)
					}
				}
			} else {
				text = "ℹ️ Unknown command. Send /help to see a list of commands."
			}
		}

		if _, err := bot.Send(updateChatID, updateMessageID, text); err != nil {
			log.Printf("Error sending message: %v", err)
		}
	}
}
