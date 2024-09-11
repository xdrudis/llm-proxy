package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

func createBatch(fileID string, auth, endpoint string) (string, error) {
	log.WithFields(log.Fields{
		"fileID":   fileID,
		"endpoint": endpoint,
	}).Debug("Creating batch")

	url := fmt.Sprintf("%s/batches", OpenAIBaseURL)
	payload := map[string]interface{}{
		"input_file_id":     fileID,
		"endpoint":          endpoint,
		"completion_window": "24h",
	}

	jsonPayload, _ := json.Marshal(payload)
	bodyContent, _, err := httpPost(url, auth, jsonPayload)
	if err != nil {
		log.WithFields(log.Fields{
			"fileID": fileID,
			"error":  err,
		}).Error("Error creating batch")
		return "", err
	}

	var batchResp BatchResponse
	err = json.Unmarshal(bodyContent, &batchResp)
	if err == nil && batchResp.Error != nil {
		log.WithFields(log.Fields{
			"fileID": fileID,
			"error":  batchResp.Error.Message,
		}).Error("OpenAI API returned an error")
		return "", errors.New(batchResp.Error.Message)
	}
	log.WithFields(log.Fields{
		"fileID":  fileID,
		"batchID": batchResp.ID,
	}).Debug("Successfully created batch")
	return batchResp.ID, err
}

func pollBatchStatus(batchID string, auth string) (*BatchResponse, error) {
	log.WithField("batchID", batchID).Debug("Starting to poll batch status")

	for {
		time.Sleep(SleepDuration)

		batchResp, err := getBatchResponse(batchID, auth)
		if err != nil {
			log.WithFields(log.Fields{
				"batchID": batchID,
				"error":   err,
			}).Error("Error getting batch response")
			return batchResp, err
		}

		// Status       Description
		// validating   the input file is being validated before the batch can begin
		// failed       the input file has failed the validation process
		// in_progress  the input file was successfully validated and the batch is currently being run
		// finalizing   the batch has completed and the results are being prepared
		// completed    the batch has been completed and the results are ready
		// expired      the batch was not able to be completed within the 24-hour time window
		// cancelling   the batch is being cancelled (may take up to 10 minutes)
		// cancelled    the batch was cancelled
		log.WithFields(log.Fields{
			"batchID":      batchID,
			"status":       batchResp.Status,
			"outputFileID": batchResp.OutputFileID,
			"errorFileID":  batchResp.ErrorFileID,
		}).Debug("Current batch status")

		switch batchResp.Status {
		case "completed", "failed", "expired", "cancelled":
			log.WithFields(log.Fields{
				"batchID":      batchID,
				"status":       batchResp.Status,
				"outputFileID": batchResp.OutputFileID,
				"errorFileID":  batchResp.ErrorFileID,
			}).Info("Batch reached final status")
			return batchResp, nil
		default:
			// Non-final states: validating, in_progress, cancelling
			log.WithFields(log.Fields{
				"batchID": batchID,
				"status":  batchResp.Status,
			}).Debug("Batch still in progress")
			time.Sleep(SleepDuration)
		}
	}
}

func getBatchResponse(batchID, auth string) (*BatchResponse, error) {
	log.WithField("batchID", batchID).Debug("Fetching batch response")

	url := fmt.Sprintf("%s/batches/%s", OpenAIBaseURL, batchID)
	data, _, err := httpGet(url, auth)
	if err != nil {
		log.WithField("batchID", batchID).Errorf("Error fetching batch response: %v", err)
		return nil, err
	}

	var batchResp BatchResponse
	err = json.Unmarshal(data, &batchResp)
	if err != nil {
		log.WithField("batchID", batchID).Errorf("Error unmarshaling batch response: %v", err)
	} else {
		log.WithFields(log.Fields{
			"batchID":      batchID,
			"status":       batchResp.Status,
			"outputFileID": batchResp.OutputFileID,
			"errorFileID":  batchResp.ErrorFileID,
		}).Debug("Successfully fetched and parsed batch response")
	}
	return &batchResp, err
}

func cancelBatch(batchID, auth string) error {
	log.WithField("batchID", batchID).Info("Attempting to cancel batch")

	url := fmt.Sprintf("%s/batches/%s/cancel", OpenAIBaseURL, batchID)
	_, _, err := httpPost(url, auth, nil)
	if err != nil {
		log.WithField("batchID", batchID).Errorf("Error cancelling batch: %v", err)
	} else {
		log.WithField("batchID", batchID).Debug("Successfully sent cancellation request")
	}
	return err
}
