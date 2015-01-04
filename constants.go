package lmail

// SMTP return codes
const (
	CodeSystemcStatus = 211
	CodeHelp          = 214
	CodeReady         = 220
	CodeClosing       = 221
	CodeOk            = 250
	CodeUserNotLocal  = 251
	CodeUserNoVerify  = 252

	CodeStartMailInput = 354

	CodeNotAvailable        = 421
	CodeMailboxNotAvailable = 450
	CodeAborted             = 451
	CodeInsufficientStorage = 452

	CodeTlsNotAvaiable = 454

	CodeNotRecognized           = 500
	CodeSyntaxError             = 501
	CodeNotImplemented          = 502
	CodeBadSequence             = 503
	CodeParameterNotImplemented = 504
	CodeNotTaken                = 550
	CodeErrUserNotLocal         = 551
	CodeMailAborted             = 552
	CodeMailboxNameNotAllowed   = 553
	CodeTransactionFailed       = 554
)

// SmtpErrors is a list of permanent negative completion messages.
var SmtpErrors = map[int]string{
	500: "Syntax Error, command not recognized",
	501: "Syntax Error in parameter or argument",
	502: "Command Not implemented",
	503: "Bad sequence of commands",
	504: "Parameter not implemented",
	550: "Requested action not taken: mailbox unavailable",
	551: "User not local; please try <forward-path>",
	552: "Requested mail action aborted: exceeded storage allocatio",
	553: "Requested action not taken: mailbox name not allowed",
	554: "Transaction failed",
}
