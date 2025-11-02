package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenAIRequest represents a simplified OpenAI API request
type OpenAIRequest struct {
	Model string `json:"model"`
}

// extractModelFromRequest extracts the model name from an OpenAI API request
func extractModelFromRequest(r *http.Request) (string, error) {
	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read request body: %w", err)
	}
	// Restore the body for later use
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse JSON
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("failed to parse request JSON: %w", err)
	}

	return req.Model, nil
}

// getCurrentModelConfig returns the current model configuration from the ConfigMap
func (as *AutoScaler) getCurrentModelConfig(ctx context.Context) (*ModelConfig, error) {
	configMap, err := as.clientset.CoreV1().ConfigMaps(as.config.Namespace).Get(
		ctx,
		as.config.ConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %w", err)
	}

	return FromConfigMapData(configMap.Data), nil
}

// updateConfigMap updates the ConfigMap with the new model configuration
func (as *AutoScaler) updateConfigMap(ctx context.Context, modelConfig *ModelConfig) error {
	configMap, err := as.clientset.CoreV1().ConfigMaps(as.config.Namespace).Get(
		ctx,
		as.config.ConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	// Update the data
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Convert model config to ConfigMap data
	for k, v := range modelConfig.ToConfigMapData() {
		configMap.Data[k] = v
	}

	_, err = as.clientset.CoreV1().ConfigMaps(as.config.Namespace).Update(
		ctx,
		configMap,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}

	log.Printf("Updated ConfigMap %s/%s with model: %s", as.config.Namespace, as.config.ConfigMapName, modelConfig.ServedModelName)
	return nil
}

// switchModelWithLock switches to the requested model with proper locking
func (as *AutoScaler) switchModelWithLock(ctx context.Context, requestedModel string) error {
	as.mu.Lock()

	// If another goroutine is already switching models, wait for it
	if as.isSwitchingModel {
		targetModel := as.switchingToModel
		as.mu.Unlock()

		// If switching to the same model, just wait
		if targetModel == requestedModel {
			log.Printf("Model switch to %s already in progress, waiting...", requestedModel)
			as.mu.Lock()
			for as.isSwitchingModel && as.switchingToModel == requestedModel {
				as.modelSwitchCond.Wait()
			}
			as.mu.Unlock()
			return nil
		}

		// Different model requested while switching - return error
		return fmt.Errorf("model switch to %s already in progress, cannot switch to %s", targetModel, requestedModel)
	}

	// We're the one switching
	as.isSwitchingModel = true
	as.switchingToModel = requestedModel
	as.mu.Unlock()

	// Ensure we clear the switching state when done
	defer func() {
		as.mu.Lock()
		as.isSwitchingModel = false
		as.switchingToModel = ""
		as.modelSwitchCond.Broadcast()
		as.mu.Unlock()
	}()

	return as.switchModel(ctx, requestedModel)
}

// switchModel switches to the requested model if it's different from the current one
func (as *AutoScaler) switchModel(ctx context.Context, requestedModel string) error {
	start := time.Now()

	// Get the model profile from CRD
	modelProfile, err := as.crdClient.GetModel(ctx, requestedModel)
	if err != nil {
		return fmt.Errorf("failed to get model from CRD: %w", err)
	}

	// Validate model config
	if err := modelProfile.Validate(); err != nil {
		return fmt.Errorf("invalid model config: %w", err)
	}

	// Get current model config
	currentConfig, err := as.getCurrentModelConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// Check if we need to switch
	if currentConfig.ServedModelName == modelProfile.ServedModelName {
		log.Printf("Model %s already loaded, no switch needed", requestedModel)
		return nil
	}

	fromModel := currentConfig.ServedModelName
	toModel := modelProfile.ServedModelName

	log.Printf("Switching from %s to %s...", fromModel, toModel)

	// Scale down to 0 first (unload current model)
	if err := as.scaleDeployment(ctx, 0); err != nil {
		return fmt.Errorf("failed to scale down: %w", err)
	}

	// Wait a bit for pods to terminate
	time.Sleep(5 * time.Second)

	// Update ConfigMap
	if err := as.updateConfigMap(ctx, modelProfile); err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}

	// Scale up to 1 (load new model)
	if err := as.scaleDeployment(ctx, 1); err != nil {
		return fmt.Errorf("failed to scale up: %w", err)
	}

	// Wait for the new deployment to be ready
	if err := as.waitForReady(ctx, as.config.GetManagedTimeout()); err != nil {
		as.metrics.RecordManagedOperation(fromModel, toModel, false, time.Since(start))
		return fmt.Errorf("failed to wait for deployment ready: %w", err)
	}

	// Record successful switch
	as.metrics.RecordManagedOperation(fromModel, toModel, true, time.Since(start))

	log.Printf("Successfully switched to model %s", modelProfile.ServedModelName)
	return nil
}
