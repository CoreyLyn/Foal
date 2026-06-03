package cli

type envelope struct {
	Command string      `json:"command"`
	Result  interface{} `json:"result,omitempty"`
	Error   *jsonError  `json:"error,omitempty"`
}

type jsonError struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Recoverable bool     `json:"recoverable"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
}

type commandResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
