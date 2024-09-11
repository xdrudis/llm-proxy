package main

type ProxyRequest struct {
	CustomID string      `json:"custom_id"`
	Method   string      `json:"method"`
	Endpoint string      `json:"url"`
	Body     interface{} `json:"body"`
}

type BatchResponse struct {
	ID            string        `json:"id"`
	Object        string        `json:"object"`
	Status        string        `json:"status"`
	OutputFileID  *string       `json:"output_file_id"`
	ErrorFileID   *string       `json:"error_file_id"`
	RequestCounts RequestCounts `json:"request_counts"`
	Error         *OpenAiError  `json:"error"`
}

type OpenAiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Param   string `json:"param,omitempty"`
	Line    *int   `json:"line,omitempty"`
}

type RequestCounts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type BatchRequestResponse struct {
	ID       string `json:"id"`
	CustomID string `json:"custom_id"`
	Response struct {
		StatusCode int         `json:"status_code"`
		RequestID  string      `json:"request_id"`
		Body       interface{} `json:"body"`
	} `json:"response"`
	Error *OpenAiError `json:"error"`
}
