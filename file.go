package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
)

func uploadFile(data []byte, auth string) (string, error) {
	url := fmt.Sprintf("%s/files", OpenAIBaseURL)

	var requestBody bytes.Buffer
	multiPartWriter := multipart.NewWriter(&requestBody)
	if err := multiPartWriter.WriteField("purpose", "batch"); err != nil {
		return "", fmt.Errorf("failed to write purpose field: %v", err)
	}

	part, err := multiPartWriter.CreateFormFile("file", "data.jsonl")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}
	if _, err = part.Write(data); err != nil {
		return "", fmt.Errorf("failed to write data to form file: %v", err)
	}

	if err = multiPartWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %v", err)
	}

	headers := map[string]string{
		"Content-Type": multiPartWriter.FormDataContentType(),
	}

	responseData, _, err := httpOp(url, "POST", auth, &requestBody, headers)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}

	var fileResponse struct {
		ID    string       `json:"id"`
		Error *OpenAiError `json:"error"`
	}
	err = json.Unmarshal(responseData, &fileResponse)
	if err == nil && fileResponse.Error != nil {
		return "", errors.New(fileResponse.Error.Message)
	}
	return fileResponse.ID, err
}

func readFile(outputFileID, auth string) ([]byte, error) {
	url := fmt.Sprintf("%s/files/%s/content", OpenAIBaseURL, outputFileID)
	d, _, e := httpGet(url, auth)
	return d, e
}

func deleteFile(fileID string, auth string) error {
	url := fmt.Sprintf("%s/files/%s", OpenAIBaseURL, fileID)
	return httpDelete(url, auth)
}
