// Package scaling provides functionality for auto-scaling Kubernetes deployments.
package scaling

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	// DefaultScaleUpTimeout is the default timeout for scaling up a deployment.
	DefaultScaleUpTimeout = 2 * time.Minute
	// DefaultCheckInterval is the default interval for checking deployment status.
	DefaultCheckInterval = 10 * time.Second
	// DefaultScaleDownDelay is the default delay before scaling down.
	DefaultScaleDownDelay = 5 * time.Minute
)

// AutoScaler interface defines the autoscaling behavior
type AutoScaler interface {
	// UpdateActivity updates the last activity timestamp
	UpdateActivity()

	// IsActive checks if the service is currently active
	IsActive() bool

	// ScaleUp scales up the deployment
	ScaleUp(ctx context.Context) error

	// ScaleDown scales down the deployment
	ScaleDown(ctx context.Context) error

	// WaitForScaleUp waits for scaling up to complete
	WaitForScaleUp(timeout time.Duration) error

	// Start starts the autoscaling loop
	Start(ctx context.Context)

	// GetStatus returns the current scaling status
	GetStatus() Status
}

// Status represents the current scaling status
type Status struct {
	IsScaledUp   bool      `json:"is_scaled_up"`
	IsScalingUp  bool      `json:"is_scaling_up"`
	LastActivity time.Time `json:"last_activity"`
	Replicas     int32     `json:"replicas"`
}

// Config holds the autoscaler configuration
type Config struct {
	Namespace      string
	Deployment     string
	ScaleDownDelay time.Duration
	CheckInterval  time.Duration
	MinReplicas    int32
	MaxReplicas    int32
}

// K8sAutoScaler implements AutoScaler using Kubernetes
type K8sAutoScaler struct {
	clientset       *kubernetes.Clientset
	config          Config
	lastActivity    time.Time
	mu              sync.RWMutex
	isScalingUp     bool
	scaleUpCond     *sync.Cond
	currentReplicas int32
}

// NewK8sAutoScaler creates a new Kubernetes autoscaler
func NewK8sAutoScaler(clientset *kubernetes.Clientset, config Config) *K8sAutoScaler {
	if config.ScaleDownDelay == 0 {
		config.ScaleDownDelay = DefaultScaleDownDelay
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = DefaultCheckInterval
	}
	if config.MinReplicas == 0 {
		config.MinReplicas = 0
	}
	if config.MaxReplicas == 0 {
		config.MaxReplicas = 1
	}

	as := &K8sAutoScaler{
		clientset:    clientset,
		config:       config,
		lastActivity: time.Now(),
	}
	as.scaleUpCond = sync.NewCond(&as.mu)
	return as
}

// UpdateActivity updates the last activity timestamp
func (as *K8sAutoScaler) UpdateActivity() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.lastActivity = time.Now()
	log.Printf("[AUTOSCALER] Activity detected at %s", as.lastActivity.Format(time.RFC3339))
}

// IsActive checks if the service has been active recently
func (as *K8sAutoScaler) IsActive() bool {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return time.Since(as.lastActivity) < as.config.ScaleDownDelay
}

// ScaleUp scales up the deployment
func (as *K8sAutoScaler) ScaleUp(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.currentReplicas >= as.config.MaxReplicas {
		log.Printf("[AUTOSCALER] Already at max replicas (%d)", as.config.MaxReplicas)
		return nil
	}

	as.isScalingUp = true
	defer func() {
		as.isScalingUp = false
		as.scaleUpCond.Broadcast()
	}()

	log.Printf("[AUTOSCALER] Scaling up deployment %s/%s", as.config.Namespace, as.config.Deployment)

	// Get current deployment
	deployment, err := as.clientset.AppsV1().Deployments(as.config.Namespace).
		Get(ctx, as.config.Deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update replicas
	replicas := as.config.MaxReplicas
	deployment.Spec.Replicas = &replicas

	// Apply the update
	_, err = as.clientset.AppsV1().Deployments(as.config.Namespace).
		Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	as.currentReplicas = replicas
	log.Printf("[AUTOSCALER] Scaled up to %d replicas", replicas)

	// Wait for deployment to be ready
	return as.waitForDeploymentReady(ctx)
}

// ScaleDown scales down the deployment
func (as *K8sAutoScaler) ScaleDown(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.currentReplicas <= as.config.MinReplicas {
		log.Printf("[AUTOSCALER] Already at min replicas (%d)", as.config.MinReplicas)
		return nil
	}

	log.Printf("[AUTOSCALER] Scaling down deployment %s/%s", as.config.Namespace, as.config.Deployment)

	// Get current deployment
	deployment, err := as.clientset.AppsV1().Deployments(as.config.Namespace).
		Get(ctx, as.config.Deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update replicas
	replicas := as.config.MinReplicas
	deployment.Spec.Replicas = &replicas

	// Apply the update
	_, err = as.clientset.AppsV1().Deployments(as.config.Namespace).
		Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	as.currentReplicas = replicas
	log.Printf("[AUTOSCALER] Scaled down to %d replicas", replicas)
	return nil
}

// WaitForScaleUp waits for the scaling operation to complete
func (as *K8sAutoScaler) WaitForScaleUp(timeout time.Duration) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.isScalingUp {
		return nil
	}

	done := make(chan struct{})
	go func() {
		as.scaleUpCond.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for scale up")
	}
}

// Start starts the autoscaling loop
func (as *K8sAutoScaler) Start(ctx context.Context) {
	go as.runScalingLoop(ctx)
}

// GetStatus returns the current scaling status
func (as *K8sAutoScaler) GetStatus() Status {
	as.mu.RLock()
	defer as.mu.RUnlock()

	return Status{
		IsScaledUp:   as.currentReplicas > as.config.MinReplicas,
		IsScalingUp:  as.isScalingUp,
		LastActivity: as.lastActivity,
		Replicas:     as.currentReplicas,
	}
}

// runScalingLoop runs the autoscaling check loop
func (as *K8sAutoScaler) runScalingLoop(ctx context.Context) {
	ticker := time.NewTicker(as.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[AUTOSCALER] Stopping autoscaling loop")
			return
		case <-ticker.C:
			as.checkAndScale(ctx)
		}
	}
}

// checkAndScale checks if scaling is needed and performs it
func (as *K8sAutoScaler) checkAndScale(ctx context.Context) {
	as.mu.RLock()
	inactive := time.Since(as.lastActivity) > as.config.ScaleDownDelay
	currentReplicas := as.currentReplicas
	as.mu.RUnlock()

	if inactive && currentReplicas > as.config.MinReplicas {
		log.Printf("[AUTOSCALER] No activity for %v, scaling down", as.config.ScaleDownDelay)
		if err := as.ScaleDown(ctx); err != nil {
			log.Printf("[AUTOSCALER] Failed to scale down: %v", err)
		}
	}
}

// waitForDeploymentReady waits for the deployment to have ready replicas
func (as *K8sAutoScaler) waitForDeploymentReady(ctx context.Context) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, DefaultScaleUpTimeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := as.clientset.AppsV1().Deployments(as.config.Namespace).
			Get(ctx, as.config.Deployment, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if all replicas are ready
		if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
			log.Printf("[AUTOSCALER] Deployment %s/%s is ready with %d replicas",
				as.config.Namespace, as.config.Deployment, deployment.Status.ReadyReplicas)
			return true, nil
		}

		log.Printf("[AUTOSCALER] Waiting for deployment: %d/%d replicas ready",
			deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
}
