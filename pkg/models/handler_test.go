package models

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/efortin/vllm-chill/pkg/kubernetes"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// MockManager implements the Manager interface for testing
type MockManager struct {
	activeModel string
	isRunning   bool
	models      []ModelInfo
	modelConfig *kubernetes.ModelConfig
	listErr     error
	configErr   error
	switchErr   error
}

func (m *MockManager) GetActiveModel() string {
	return m.activeModel
}

func (m *MockManager) SwitchModel(ctx context.Context, modelID string) error {
	if m.switchErr != nil {
		return m.switchErr
	}
	m.activeModel = modelID
	return nil
}

func (m *MockManager) GetModelConfig(ctx context.Context, modelID string) (*kubernetes.ModelConfig, error) {
	if m.configErr != nil {
		return nil, m.configErr
	}
	return m.modelConfig, nil
}

func (m *MockManager) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.models, nil
}

func (m *MockManager) IsRunning() bool {
	return m.isRunning
}

func TestNewHandler(t *testing.T) {
	mockManager := &MockManager{}
	handler := NewHandler(mockManager)
	assert.NotNil(t, handler)
	assert.Equal(t, mockManager, handler.manager)
}

func TestHandler_AvailableHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		mockModels     []ModelInfo
		mockErr        error
		expectedStatus int
	}{
		{
			name: "successful list",
			mockModels: []ModelInfo{
				{Name: "model1", ServedModelName: "model1", ModelName: "test/model1"},
				{Name: "model2", ServedModelName: "model2", ModelName: "test/model2"},
			},
			mockErr:        nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "list error",
			mockModels:     nil,
			mockErr:        errors.New("failed to list"),
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "empty list",
			mockModels:     []ModelInfo{},
			mockErr:        nil,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &MockManager{
				models:  tt.mockModels,
				listErr: tt.mockErr,
			}
			handler := NewHandler(mockManager)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/models/available", nil)

			handler.AvailableHandler(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandler_RunningHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		activeModel    string
		isRunning      bool
		modelConfig    *kubernetes.ModelConfig
		configErr      error
		expectedStatus int
	}{
		{
			name:        "successful running model",
			activeModel: "test-model",
			isRunning:   true,
			modelConfig: &kubernetes.ModelConfig{
				ModelName:       "test/model",
				ServedModelName: "test-model",
				MaxModelLen:     "4096",
				ToolCallParser:  "hermes",
				ReasoningParser: "deepseek_r1",
			},
			configErr:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "config error",
			activeModel:    "test-model",
			isRunning:      false,
			modelConfig:    nil,
			configErr:      errors.New("failed to get config"),
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "model not running",
			activeModel: "",
			isRunning:   false,
			modelConfig: &kubernetes.ModelConfig{
				ModelName:       "test/model",
				ServedModelName: "",
			},
			configErr:      nil,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &MockManager{
				activeModel: tt.activeModel,
				isRunning:   tt.isRunning,
				modelConfig: tt.modelConfig,
				configErr:   tt.configErr,
			}
			handler := NewHandler(mockManager)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/models/running", nil)

			handler.RunningHandler(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
