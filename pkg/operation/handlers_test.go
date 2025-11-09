package operation_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/operation"
)

var _ = Describe("Handler", func() {
	var (
		handler     *operation.Handler
		mockManager *MockManager
	)

	BeforeEach(func() {
		mockManager = &MockManager{}
		handler = operation.NewHandler(mockManager)
	})

	Describe("StartHandler", func() {
		Context("when using POST method", func() {
			Context("and start succeeds", func() {
				It("should return success response", func() {
					req := httptest.NewRequest(http.MethodPost, "/operations/start", nil)
					w := httptest.NewRecorder()

					handler.StartHandler(w, req)

					Expect(w.Code).To(Equal(http.StatusOK))
					Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
					Expect(w.Body.String()).To(ContainSubstring("success"))
					Expect(w.Body.String()).To(ContainSubstring("vLLM started successfully"))
					Expect(mockManager.startCalled).To(BeTrue())
					Expect(mockManager.activityCalled).To(BeTrue())
				})
			})

			Context("and start fails", func() {
				BeforeEach(func() {
					mockManager.startError = errors.New("deployment not found")
				})

				It("should return error response", func() {
					req := httptest.NewRequest(http.MethodPost, "/operations/start", nil)
					w := httptest.NewRecorder()

					handler.StartHandler(w, req)

					Expect(w.Code).To(Equal(http.StatusInternalServerError))
					Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
					Expect(w.Body.String()).To(ContainSubstring("error"))
					Expect(w.Body.String()).To(ContainSubstring("start_failed"))
					Expect(mockManager.startCalled).To(BeTrue())
				})
			})
		})

		Context("when using wrong HTTP method", func() {
			It("should return method not allowed for GET", func() {
				req := httptest.NewRequest(http.MethodGet, "/operations/start", nil)
				w := httptest.NewRecorder()

				handler.StartHandler(w, req)

				Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
				Expect(mockManager.startCalled).To(BeFalse())
			})

			It("should return method not allowed for PUT", func() {
				req := httptest.NewRequest(http.MethodPut, "/operations/start", nil)
				w := httptest.NewRecorder()

				handler.StartHandler(w, req)

				Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
				Expect(mockManager.startCalled).To(BeFalse())
			})
		})
	})

	Describe("StopHandler", func() {
		Context("when using POST method", func() {
			Context("and stop succeeds", func() {
				It("should return success response", func() {
					req := httptest.NewRequest(http.MethodPost, "/operations/stop", nil)
					w := httptest.NewRecorder()

					handler.StopHandler(w, req)

					Expect(w.Code).To(Equal(http.StatusOK))
					Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
					Expect(w.Body.String()).To(ContainSubstring("success"))
					Expect(w.Body.String()).To(ContainSubstring("vLLM stopped successfully"))
					Expect(mockManager.stopCalled).To(BeTrue())
				})
			})

			Context("and stop fails", func() {
				BeforeEach(func() {
					mockManager.stopError = errors.New("failed to update deployment")
				})

				It("should return error response", func() {
					req := httptest.NewRequest(http.MethodPost, "/operations/stop", nil)
					w := httptest.NewRecorder()

					handler.StopHandler(w, req)

					Expect(w.Code).To(Equal(http.StatusInternalServerError))
					Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
					Expect(w.Body.String()).To(ContainSubstring("error"))
					Expect(w.Body.String()).To(ContainSubstring("stop_failed"))
					Expect(mockManager.stopCalled).To(BeTrue())
				})
			})
		})

		Context("when using wrong HTTP method", func() {
			It("should return method not allowed for GET", func() {
				req := httptest.NewRequest(http.MethodGet, "/operations/stop", nil)
				w := httptest.NewRecorder()

				handler.StopHandler(w, req)

				Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
				Expect(mockManager.stopCalled).To(BeFalse())
			})

			It("should return method not allowed for DELETE", func() {
				req := httptest.NewRequest(http.MethodDelete, "/operations/stop", nil)
				w := httptest.NewRecorder()

				handler.StopHandler(w, req)

				Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
				Expect(mockManager.stopCalled).To(BeFalse())
			})
		})
	})
})
