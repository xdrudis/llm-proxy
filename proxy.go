package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	OpenAIBaseURL = "https://api.openai.com/v1"
	SleepDuration = 5 * time.Second
)

type batchKey struct {
	auth     string
	endpoint string
}

var (
	port              = 3030
	maxHoldBatchSend  = 4 * time.Second
	maxBatchSize      = 1000   // OpenAI supports 50k, but tail latencies could be massive
	maxBatchMb        = 25     // OpenAI supports up to 100 MB
	reqToBeBatchedMap sync.Map // key: batchKey, value: chan ProxyRequest
	shutdownChan      = make(chan struct{})
	responseChanMap   sync.Map // key: customID (id of a request), value: channel for the response
	batchMap          sync.Map // key: BatchResponse.ID, value: auth. So that we can cancel them on ctrl-c
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	log.SetOutput(os.Stderr)
	log.SetLevel(log.InfoLevel)
}

// go run . -port 8080 -max-hold-batch 5s -max-batch-size 500 -max-batch-mb 25
func main() {
	flag.IntVar(&port, "port", port, "Port to run the server on")
	flag.DurationVar(&maxHoldBatchSend, "max-hold-batch", maxHoldBatchSend, "Maximum time to hold a batch before sending")
	flag.IntVar(&maxBatchSize, "max-batch-size", maxBatchSize, "Maximum number of requests in a batch")
	flag.IntVar(&maxBatchMb, "max-batch-mb", maxBatchMb, "Maximum size of a batch in bytes")
	flag.Parse()

	log.Info("Starting server with maxHoldBatchSend: ", maxHoldBatchSend, ", maxBatchSize: ", maxBatchSize, ", maxBatchMb: ", maxBatchMb)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: createMuxServer(),
	}

	go func() {
		log.Infof("Server is running on :%d", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop

	log.Info("Shutting down server...")

	// signal all goroutines to stop
	close(shutdownChan)

	cancelAllOutstandingBatches()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("Server exiting")
}

func createMuxServer() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleOpenaiPostEndpoint)
	mux.HandleFunc("/v1/embeddings", handleOpenaiPostEndpoint)
	mux.HandleFunc("/stats", handleStats)
	mux.HandleFunc("/", handleNoopOpenaiProxy)
	return mux
}

func cancelAllOutstandingBatches() {
	var wg sync.WaitGroup

	batchMap.Range(func(key, value interface{}) bool {
		batchID := key.(string)
		auth := value.(string)

		wg.Add(1)
		safeGo2(func(id, auth string) {
			defer wg.Done()
			log.Printf("Cancelling batch %s", id)
			if err := cancelBatch(id, auth); err != nil {
				log.Printf("Error cancelling batch %s: %v", id, err)
			}
			// http requests to this proxy in the batch will error out when the server shuts down
		})(batchID, auth)

		return true
	})

	// Wait for all cancellations
	wg.Wait()
}

func handleOpenaiPostEndpoint(w http.ResponseWriter, r *http.Request) {
	trackRequestStart()
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	customID := fmt.Sprintf("req_%d", rand.Intn(1000000))
	log.WithField("requestID", customID).Debugf("New request received for endpoint: %s", r.URL.Path)

	responseChan := make(chan interface{})
	responseChanMap.Store(customID, responseChan)
	defer responseChanMap.Delete(customID)

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	key := batchKey{
		auth:     r.Header.Get("Authorization"),
		endpoint: r.URL.Path,
	}

	req := ProxyRequest{
		CustomID: customID,
		Method:   "POST",
		Endpoint: r.URL.Path,
		Body:     bodyMap,
	}

	value, loaded := reqToBeBatchedMap.LoadOrStore(key, make(chan ProxyRequest, 1))
	ch := value.(chan ProxyRequest)
	if !loaded {
		log.Printf("[%s] Created a new channel for %+v", customID, key)
		safeGo2(processUploadAndCreateBatch)(key, ch)
	}
	ch <- req
	log.WithField("requestID", customID).Debug("Request sent to be batched")

	response := <-responseChan
	log.WithField("requestID", customID).Debug("Received response from batch")

	trackRequestEnd(true, time.Since(start))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := getStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func processUploadAndCreateBatch(key batchKey, reqToBeBatched chan ProxyRequest) {
	var batch []ProxyRequest
	var jsonlData bytes.Buffer
	batchSize := 0
	batchBytes := 0
	maxBatchBytes := maxBatchMb * 1024 * 1024
	batchStart := time.Now()

	log.Printf("[Batch] Starting new batch for key %+v", key)

	for {
		select {
		case req := <-reqToBeBatched:
			log.WithFields(log.Fields{
				"requestID": req.CustomID,
				"key":       key,
			}).Debug("New request added to batch")
			jsonReq, err := json.Marshal(req)
			if err != nil {
				log.Printf("[Batch] Failed to marshal proxy request: %v", err)
				continue
			}

			jsonReq = append(jsonReq, '\n') // JSONL: each JSON in a new line

			if batchSize >= maxBatchSize || batchBytes+len(jsonReq) > maxBatchBytes || len(batch) == 0 && len(jsonReq) > maxBatchBytes {
				if len(batch) > 0 {
					log.Printf("[Batch] Batch full, processing %d requests", len(batch))
					safeGo4(processBatch)(jsonlData.Bytes(), key.auth, key.endpoint, outstandingCustomIDs(batch))
					batch = nil
					jsonlData.Reset()
					batchSize = 0
					batchBytes = 0
					batchStart = time.Now()
				}
			}

			batch = append(batch, req)
			jsonlData.Write(jsonReq)
			batchSize++
			batchBytes += len(jsonReq)

			log.WithFields(log.Fields{
				"batchSize":  batchSize,
				"batchBytes": batchBytes,
			}).Debug("Current batch status")

		case <-time.After(200 * time.Millisecond):
			if len(batch) > 0 && (time.Since(batchStart) >= maxHoldBatchSend || batchSize >= maxBatchSize) {
				log.WithFields(log.Fields{
					"requests":       len(batch),
					"timeSinceStart": time.Since(batchStart),
				}).Info("Processing batch due to time or size limit")
				safeGo4(processBatch)(jsonlData.Bytes(), key.auth, key.endpoint, outstandingCustomIDs(batch))
				batch = nil
				jsonlData.Reset()
				batchSize = 0
				batchBytes = 0
				batchStart = time.Now()
			}

		case <-shutdownChan:
			log.Info("Received shutdown signal")
			if len(batch) > 0 {
				log.WithField("requests", len(batch)).Info("Processing final batch before shutdown")
				safeGo4(processBatch)(jsonlData.Bytes(), key.auth, key.endpoint, outstandingCustomIDs(batch))
			}
			reqToBeBatchedMap.Delete(key)
			return
		}
	}
}

func processBatch(jsonlData []byte, auth, endpoint string, outstandingCustomIDs map[string]bool) {
	trackBatchStart()
	start := time.Now()
	log.WithField("requests", len(outstandingCustomIDs)).Info("Starting to process batch")

	fileID, err := uploadFile(jsonlData, auth)
	if err != nil {
		log.WithError(err).Error("Failed to upload file to OpenAI")
		sendErrorToAllRequests(outstandingCustomIDs, fmt.Sprintf("Failed to upload file: %v", err))
		trackBatchEnd(false, time.Since(start))
		return
	}
	log.WithField("fileID", fileID).Info("File uploaded successfully")

	batchID, err := createBatch(fileID, auth, endpoint)
	if err != nil {
		log.Printf("[ProcessBatch] Failed to create batch: %v", err)
		if err := deleteFile(fileID, auth); err != nil {
			log.Printf("[ProcessBatch] Warning: Failed to delete input file: %v", err)
		}
		sendErrorToAllRequests(outstandingCustomIDs, fmt.Sprintf("Failed to create batch: %v", err))
		trackBatchEnd(false, time.Since(start))
		return
	}
	log.Printf("[ProcessBatch] Batch created successfully, ID: %s", batchID)

	// Store the batch ID and headers for potential cancellation
	batchMap.Store(batchID, auth)

	safeGo4(processBatchResponse)(batchID, auth, outstandingCustomIDs, start)
}

func processBatchResponse(batchID, auth string, outstandingCustomIDs map[string]bool, start time.Time) {
	defer batchMap.Delete(batchID)

	log.WithField("batchID", batchID).Info("Starting to process batch response")

	batchResponse, err := pollBatchStatus(batchID, auth)
	if err != nil {
		log.WithError(err).Error("Failed batch or batch status")
		sendErrorToAllRequests(outstandingCustomIDs, fmt.Sprintf("Batch processing failed: %v", err))
		trackBatchEnd(false, time.Since(start))
		return
	}
	log.WithFields(log.Fields{
		"batchID":      batchID,
		"status":       batchResponse.Status,
		"outputFileID": batchResponse.OutputFileID,
		"errorFileID":  batchResponse.ErrorFileID,
	}).Info("Batch status received")

	filesToProcess := []*string{batchResponse.OutputFileID, batchResponse.ErrorFileID}

	var waitDelete sync.WaitGroup
	defer waitDelete.Wait()
	for _, fileID := range filesToProcess {
		if fileID == nil {
			continue
		}

		jsonlContent, err := readFile(*fileID, auth)
		if err != nil {
			log.Printf("[ProcessBatchResponse] Failed to retrieve file %s: %v", *fileID, err)
			continue
		}
		log.Printf("[ProcessBatchResponse] Successfully retrieved file %s. Content length: %d", *fileID, len(jsonlContent))

		waitDelete.Add(1)
		safeGo1(func(id string) {
			defer waitDelete.Done()
			if err := deleteFile(id, auth); err != nil {
				log.Printf("[ProcessBatchResponse] Warning: Failed to delete file %s: %v", id, err)
			}
		})(*fileID)

		processFileContent(jsonlContent, outstandingCustomIDs)
	}

	// Send error responses for any remaining outstanding requests. Shouldn't happen
	for customID := range outstandingCustomIDs {
		log.Printf("[ProcessBatchResponse] Sending error response for outstanding request ID: %s", customID)
		sendErrorResponse(customID, "No response received for request ["+customID+"] in the batch")
	}

	trackBatchEnd(true, time.Since(start))
	log.WithField("batchID", batchID).Info("Finished processing batch response")
}

func processFileContent(jsonlContent []byte, outstandingCustomIDs map[string]bool) {
	for _, line := range bytes.Split(jsonlContent, []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		var reqResponse BatchRequestResponse
		if err := json.Unmarshal(line, &reqResponse); err != nil {
			log.Printf("[ProcessFileContent] Failed to parse batch output line: %v", err)
			continue
		}

		log.Printf("[ProcessFileContent] Processing response for request ID: %s", reqResponse.CustomID)

		if ch, ok := responseChanMap.Load(reqResponse.CustomID); ok {
			if reqResponse.Error != nil {
				ch.(chan interface{}) <- map[string]interface{}{
					"error": reqResponse.Error,
				}
			} else {
				ch.(chan interface{}) <- reqResponse.Response.Body
			}
			close(ch.(chan interface{}))
			delete(outstandingCustomIDs, reqResponse.CustomID)
			log.Printf("[ProcessFileContent] Response sent for request ID: %s", reqResponse.CustomID)
		} else {
			log.Printf("[ProcessFileContent] No waiting request found for CustomID: %s", reqResponse.CustomID)
		}
	}
}

func outstandingCustomIDs(batch []ProxyRequest) map[string]bool {
	m := make(map[string]bool)
	for _, req := range batch {
		m[req.CustomID] = true
	}
	return m
}

// Helper function to send error response for an individual request
func sendErrorResponse(customID string, errorMsg string) {
	log.Printf("[ErrorResponse] Sending error response for request ID: %s, Error: %s", customID, errorMsg)
	if ch, ok := responseChanMap.Load(customID); ok {
		ch.(chan interface{}) <- map[string]interface{}{
			"error": map[string]string{
				"message": errorMsg,
			},
		}
		close(ch.(chan interface{}))
		log.Printf("[ErrorResponse] Error response sent and channel closed for request ID: %s", customID)
	} else {
		log.Printf("[ErrorResponse] No response channel found for request ID: %s\n", customID)
	}
	trackSynthesizedErrorResponse()
}

// Helper function to send error responses for all requests in a batch
func sendErrorToAllRequests(customIDs map[string]bool, errorMsg string) {
	log.Printf("[BatchError] Sending error to %d requests: %s", len(customIDs), errorMsg)
	for customID := range customIDs {
		sendErrorResponse(customID, errorMsg)
	}
}

// any other endpoint we don't handle, forward transparently
func handleNoopOpenaiProxy(w http.ResponseWriter, r *http.Request) {
	log.WithField("path", r.URL.Path).Info("Forwarding request to OpenAI")
	openAIURL := "https://api.openai.com" + r.URL.Path

	proxyReq, err := http.NewRequest(r.Method, openAIURL, r.Body)
	if err != nil {
		log.Printf("[NoopProxy] Error creating proxy request: %v", err)
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	for name, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}
	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("[NoopProxy] Error forwarding request to OpenAI: %v", err)
		http.Error(w, "Error forwarding request to OpenAI", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[NoopProxy] Error copying response body: %v", err)
	}
	log.WithField("path", r.URL.Path).Info("Successfully forwarded request and received response")
}
