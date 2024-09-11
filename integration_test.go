package main

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxyIntegration(t *testing.T) {
	proxyServer := httptest.NewServer(createMuxServer())
	defer proxyServer.Close()

	var wg sync.WaitGroup

	auth := "Bearer " + os.Getenv("OPENAI_API_KEY")

	makeChatRequest := func(prompt string) {
		payload := map[string]interface{}{
			"model":      "gpt-4o-mini",
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
			"max_tokens": 5,
		}
		jsonPayload, _ := json.Marshal(payload)
		wg.Add(1)
		go func(prompt string) {
			fmt.Println("Sending " + prompt)
			defer wg.Done()

			data, _, err := httpPost(proxyServer.URL+"/v1/chat/completions", auth, jsonPayload)
			if err != nil {
				t.Fatalf("Failed to make chat completion request: %v", err)
			}
			fmt.Println("Received response for " + prompt + ": " + strings.TrimSpace(string(data)))
		}(prompt)
	}

	makeEmbeddingsRequest := func(input string) {
		payload := map[string]interface{}{
			"model": "text-embedding-3-small",
			"input": input,
		}
		jsonPayload, _ := json.Marshal(payload)
		wg.Add(1)
		go func(prompt string) {
			fmt.Println("Sending embedding " + input)
			defer wg.Done()

			data, _, err := httpPost(proxyServer.URL+"/v1/embeddings", auth, jsonPayload)
			if err != nil {
				t.Fatalf("Failed to make chat completion request: %v", err)
			}
			fmt.Println("Received response for " + input + ": " + strings.TrimSpace(string(data)))
		}(input)
	}

	getStats := func() Stats {
		resp, err := http.Get(proxyServer.URL + "/stats")
		if err != nil {
			t.Fatalf("Failed to get stats: %v", err)
		}
		defer resp.Body.Close()
		var stats Stats
		json.NewDecoder(resp.Body).Decode(&stats)
		return stats
	}

	makeChatRequest("Say Hi")
	makeChatRequest("Say Aye")

	time.Sleep(100 * time.Millisecond)
	stats := getStats()
	assert.Equal(t, int64(2), stats.Requests.Total)
	assert.Equal(t, int64(0), stats.Requests.Successful)
	assert.Equal(t, int64(0), stats.Batches.Total) // batch shouldn't have been created yet

	time.Sleep(maxHoldBatchSend + 250*time.Millisecond) // force starting of a new batch

	makeChatRequest("Say Boom")
	makeEmbeddingsRequest("text") // shouldn't batch with Say boom because it's a different endpoint
	wg.Wait()

	stats = getStats()
	assert.Equal(t, int64(4), stats.Requests.Successful)
	assert.Equal(t, int64(3), stats.Batches.Successful)
	assert.Equal(t, int64(0), stats.Requests.Failed)
	assert.Equal(t, int64(0), stats.Batches.Failed)

	time.Sleep(250 * time.Millisecond) // give time for the last delete file to succeed
}
