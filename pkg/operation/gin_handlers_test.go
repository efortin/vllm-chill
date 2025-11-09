package operation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/operation"
	"github.com/gin-gonic/gin"
)

// MockManager implements the operation.Manager interface for testing
type MockManager struct {
	startError     error
	stopError      error
	activityCalled bool
	startCalled    bool
	stopCalled     bool
}

func (m *MockManager) Start(_ context.Context) error {
	m.startCalled = true
	return m.startError
}

func (m *MockManager) Stop(_ context.Context) error {
	m.stopCalled = true
	return m.stopError
}

func (m *MockManager) UpdateActivity() {
	m.activityCalled = true
}

var _ = Describe("GinHandler", func() {
	var (
		handler     *operation.GinHandler
		mockManager *MockManager
		router      *gin.Engine
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		mockManager = &MockManager{}
		handler = operation.NewGinHandler(mockManager)
		router = gin.New()
		router.POST("/operations/start", handler.StartHandler)
		router.POST("/operations/stop", handler.StopHandler)
	})

	Describe("StartHandler", func() {
		Context("when start succeeds", func() {
			It("should return success response", func() {
				req := httptest.NewRequest(http.MethodPost, "/operations/start", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(w.Body.String()).To(ContainSubstring("success"))
				Expect(w.Body.String()).To(ContainSubstring("vLLM started successfully"))
				Expect(mockManager.startCalled).To(BeTrue())
				Expect(mockManager.activityCalled).To(BeTrue())
			})
		})

		Context("when start fails", func() {
			BeforeEach(func() {
				mockManager.startError = errors.New("failed to scale up")
			})

			It("should return error response", func() {
				req := httptest.NewRequest(http.MethodPost, "/operations/start", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusInternalServerError))
				Expect(w.Body.String()).To(ContainSubstring("error"))
				Expect(w.Body.String()).To(ContainSubstring("start_failed"))
				Expect(mockManager.startCalled).To(BeTrue())
			})
		})
	})

	Describe("StopHandler", func() {
		Context("when stop succeeds", func() {
			It("should return success response", func() {
				req := httptest.NewRequest(http.MethodPost, "/operations/stop", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(w.Body.String()).To(ContainSubstring("success"))
				Expect(w.Body.String()).To(ContainSubstring("vLLM stopped successfully"))
				Expect(mockManager.stopCalled).To(BeTrue())
			})
		})

		Context("when stop fails", func() {
			BeforeEach(func() {
				mockManager.stopError = errors.New("failed to scale down")
			})

			It("should return error response", func() {
				req := httptest.NewRequest(http.MethodPost, "/operations/stop", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusInternalServerError))
				Expect(w.Body.String()).To(ContainSubstring("error"))
				Expect(w.Body.String()).To(ContainSubstring("stop_failed"))
				Expect(mockManager.stopCalled).To(BeTrue())
			})
		})
	})
})
