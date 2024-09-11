package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

var httpClient = &http.Client{}

func httpGet(inputUrl, auth string) (data []byte, status int, err error) {
	return httpOp(inputUrl, "GET", auth, nil, nil)
}

func httpPost(inputUrl, auth string, body []byte) (data []byte, status int, err error) {
	return httpOp(inputUrl, "POST", auth, bytes.NewReader(body), nil)
}

func httpDelete(inputUrl, auth string) error {
	_, _, err := httpOp(inputUrl, "DELETE", auth, nil, nil)
	return err
}

func httpOp(inputUrl, op, auth string, body io.Reader, additionalHeaders map[string]string) (data []byte, status int, err error) {
	const maxRetries = 3
	const userAgent = "github.com/xdrudis/llm-proxy"

	req, err := http.NewRequest(op, inputUrl, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Encoding", "gzip")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	for key, value := range additionalHeaders {
		req.Header.Set(key, value)
	}

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(math.Pow(1.5, float64(i))) * time.Second)
		}

		var resp *http.Response
		if resp, err = httpClient.Do(req); err != nil {
			continue
		}

		status = resp.StatusCode
		reader := resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			var gzipReader *gzip.Reader
			gzipReader, err = gzip.NewReader(resp.Body)
			if err != nil {
				resp.Body.Close()
				return nil, status, err
			}
			defer gzipReader.Close()
			reader = gzipReader
		}

		data, err = io.ReadAll(reader)
		resp.Body.Close()
		if err != nil {
			continue
		}

		if isRetriable(status) {
			_ = resp.Body.Close()
			if i == maxRetries-1 { // exhausted retries
				return nil, status, fmt.Errorf("HTTP status code %d received: %s", status, string(data))
			}
			continue
		} else if status < 200 || status >= 300 {
			_ = resp.Body.Close()
			return nil, status, fmt.Errorf("HTTP non-retriable status code %d received: %s", status, string(data))
		}

		return data, status, err
	}
	return nil, -1, err
}

func isRetriable(httpStatusCode int) bool {
	switch httpStatusCode {
	case 408, // Request Timeout
		429, // Too Many Requests
		500, // Internal Server Error
		502, // Bad Gateway
		503, // Service Unavailable
		504: // Gateway Timeout
		return true
	default:
		// Check for 5xx status codes (server errors)
		return httpStatusCode >= 500 && httpStatusCode < 600
	}
}
