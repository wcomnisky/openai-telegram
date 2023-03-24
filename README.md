# GPT4-bot

> Chat with the latest OpenAI language model GPT-4. 

Go CLI to fuels a Telegram bot that lets you interact with [GPT4](https://openai.com/product/gpt-4), a large language model trained by OpenAI.

## Installation

Clone this repository and run `go build` in its root directory. 

After you download the file, extract it into a folder and open the `env.example` file with a text editor and fill in your credentials. 
- `TELEGRAM_TOKEN`: Your Telegram Bot token
  - Follow [this guide](https://core.telegram.org/bots/tutorial#obtain-your-bot-token) to create a bot and get the token.
- `OPENAI_KEY`: your OpenAI API key. You will need to be invited by OpenAI to have access to the GPT4 model. 
- `TELEGRAM_ID` (Optional): Your Telegram User ID
  - If you set this, only you will be able to interact with the bot.
  - To get your ID, message `@userinfobot` on Telegram.
  - Multiple IDs can be provided, separated by commas.
- `EDIT_WAIT_SECONDS` (Optional): Amount of seconds to wait between edits
  - This is set to `1` by default, but you can increase if you start getting a lot of `Too Many Requests` errors.
- Save the file, and rename it to `.env`.
> **Note** Make sure you rename the file to _exactly_ `.env`! The program won't work otherwise.

Finally, run `./openai-telegram`.

## Usage of the Telegram bot

Directly send your message and wait for the reply!
In addition, add "SYSTEM:" before your message to set the system information of your GPT-4 bot. 

Commands:
- /start: start the bot
- /help: get help information
- /reset: clear the conversation history 
