package domain

// Command is an intent to change a payment's state. The API layer translates
// each incoming request into exactly one command; the transition function
// (added in the next step) decides whether that command is legal from the
// payment's current state.
type Command string

const (
	// CmdAuthorize: attempt to authorize an initiated payment.
	CmdAuthorize Command = "authorize"
	// CmdCapture: capture an authorized payment.
	CmdCapture Command = "capture"
	// CmdSettle: settle a captured payment.
	CmdSettle Command = "settle"
	// CmdRefund: refund a captured or settled payment.
	CmdRefund Command = "refund"
	// CmdExpire: mark an authorization as lapsed.
	CmdExpire Command = "expire"
	// CmdFail: mark a payment as failed.
	CmdFail Command = "fail"
	// CmdFlagHold: the risk layer requests a hold on an authorized payment.
	CmdFlagHold Command = "flag_hold"
	// CmdClearHold: a held payment is cleared and proceeds.
	CmdClearHold Command = "clear_hold"
)

// AllCommands lists every command the core accepts.
var AllCommands = []Command{
	CmdAuthorize,
	CmdCapture,
	CmdSettle,
	CmdRefund,
	CmdExpire,
	CmdFail,
	CmdFlagHold,
	CmdClearHold,
}
