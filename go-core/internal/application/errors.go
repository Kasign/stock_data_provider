package application

// InputError identifies a caller-supplied value that fails workflow validation.
// Transport layers can distinguish it from provider/LLM/storage failures.
type InputError struct {
	Message string
}

func (e *InputError) Error() string { return e.Message }
