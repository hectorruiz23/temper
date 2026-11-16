package kernel

type RuleError struct {
	Code    string
	Message string
}

func NewRuleError(code, message string) RuleError {
	return RuleError{Code: code, Message: message}
}

func (e RuleError) Error() string {
	return e.Message
}
