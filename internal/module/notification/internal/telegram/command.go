package telegram

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

// commandEntity returns the bot_command entity when the message starts with
// one. The bounds check guards against malformed entities in untrusted
// webhook payloads.
func commandEntity(msg *models.Message) (models.MessageEntity, bool) {
	if msg == nil || len(msg.Entities) == 0 {
		return models.MessageEntity{}, false
	}
	entity := msg.Entities[0]
	if entity.Type != models.MessageEntityTypeBotCommand || entity.Offset != 0 ||
		entity.Length <= 0 || entity.Length > len(msg.Text) {
		return models.MessageEntity{}, false
	}
	return entity, true
}

// messageCommand extracts the command name without the leading slash or the
// @botname mention suffix. It returns "" when the message is not a command.
func messageCommand(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok {
		return ""
	}
	command := msg.Text[1:entity.Length]
	if at := strings.Index(command, "@"); at != -1 {
		command = command[:at]
	}
	return command
}

// commandArguments returns the text following the command, or "" when the
// command has no arguments.
func commandArguments(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok || entity.Length >= len(msg.Text) {
		return ""
	}
	return msg.Text[entity.Length+1:]
}

// shortcutCommands are the commands whose first argument is a number — a
// ticket, subscription or user id, or a page — and which the bot's own
// listings therefore offer as "/<command>_<n>" shortcuts: a Telegram command
// cannot contain a space, so the number rides in the command name to make it
// a single tap.
var shortcutCommands = []string{
	"tk", "rp", "close", "reopen", "tickets",
	"user", "user_sub", "user_log", "reset", "toggle", "ban",
}

// splitShortcut splits "/user_sub_9" into the command "user_sub" and the
// number "9". Command names contain underscores themselves, so the longest
// known command the shortcut starts with wins over the first underscore.
func splitShortcut(command string) (name, number string, ok bool) {
	for _, candidate := range shortcutCommands {
		rest, found := strings.CutPrefix(command, candidate+"_")
		if !found || rest == "" || strings.Trim(rest, "0123456789") != "" || len(candidate) <= len(name) {
			continue
		}
		name, number, ok = candidate, rest, true
	}
	return name, number, ok
}

// expandShortcut turns the shortcut "/tk_12" into the command "tk" with the
// arguments "12"; arguments after the shortcut ("/rp_12 text") follow the id.
// Anything else is returned unchanged.
func expandShortcut(command, args string) (string, string) {
	name, number, ok := splitShortcut(command)
	if !ok {
		return command, args
	}
	if args == "" {
		return name, number
	}
	return name, number + " " + args
}
