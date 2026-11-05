package telegram

import "context"

// Command describes one entry of the bot's command menu. It mirrors
// Telegram's BotCommand without binding this package to the bot library.
type Command struct {
	Command     string
	Description string
}

// TelegramCommandRegistrar publishes the command menu Telegram shows in the
// message composer. A zero chatID addresses the default scope, which every
// user sees; a non-zero chatID scopes the menu to that chat.
// SetGroupAdminCommands scopes the menu to a group's administrators, which
// is how the administrator commands stay invisible to ordinary members.
type TelegramCommandRegistrar interface {
	SetCommands(ctx context.Context, chatID int64, commands []Command) error
	SetGroupAdminCommands(ctx context.Context, chatID int64, commands []Command) error
}

var (
	startCommand   = Command{Command: "start", Description: "绑定账号"}
	bindCommand    = Command{Command: "bind", Description: "使用绑定令牌绑定账号：/bind <token>"}
	trafficCommand = Command{Command: "traffic", Description: "查看订阅流量"}
	helpCommand    = Command{Command: "help", Description: "查看可用命令"}
)

// PublicCommands is the menu every user gets: account binding, the traffic
// of the bound account, plus the help entry that makes the administrator
// surface discoverable to the people who have access to it. Handlers still
// authenticate, so a non-administrator only learns that the command exists.
func PublicCommands() []Command {
	return []Command{startCommand, bindCommand, trafficCommand, helpCommand}
}

// AdminCommands is the menu published to the administrators of the admin
// group (chat_administrators scope), where the administrator commands are
// the only place they work. It deliberately excludes /confirm_* and
// /cancel_* because those carry a generated action id and are only ever
// offered inline, and /traffic, which only answers in the private chat.
func AdminCommands() []Command {
	return append([]Command{startCommand, bindCommand, helpCommand},
		Command{Command: "dash", Description: "数据看板"},
		Command{Command: "tickets", Description: "工单列表：/tickets [页码]"},
		Command{Command: "tickets_waiting", Description: "待处理工单"},
		Command{Command: "tk", Description: "工单详情：/tk <工单号>"},
		Command{Command: "rp", Description: "回复工单：/rp <工单号> <内容>"},
		Command{Command: "close", Description: "关闭工单：/close <工单号>"},
		Command{Command: "reopen", Description: "重开工单：/reopen <工单号>"},
		Command{Command: "user", Description: "用户详情：/user <邮箱或ID>"},
		Command{Command: "user_sub", Description: "用户订阅：/user_sub <邮箱或ID>"},
		Command{Command: "user_log", Description: "用户日志：/user_log <邮箱或ID>"},
		Command{Command: "reset", Description: "重置流量：/reset <订阅ID>"},
		Command{Command: "toggle", Description: "启停订阅：/toggle <订阅ID>"},
		Command{Command: "ban", Description: "封禁用户：/ban <邮箱或ID>"},
	)
}
