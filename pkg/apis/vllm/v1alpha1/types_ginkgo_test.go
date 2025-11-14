package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/apis/vllm/v1alpha1"
)

var _ = Describe("VLLM Model Types", func() {
	Context("VLLMModel Struct", func() {
		It("should instantiate VLLMModel struct without error", func() {
			// Test that the VLLMModel struct exists and can be instantiated
			model := v1alpha1.VLLMModel{}
			Expect(model).NotTo(BeNil())
		})
	})

	Context("VLLMModelSpec Struct", func() {
		It("should instantiate VLLMModelSpec struct without error", func() {
			// Test that the VLLMModelSpec struct exists and can be instantiated
			spec := v1alpha1.VLLMModelSpec{}
			Expect(spec).NotTo(BeNil())
		})
	})

	Context("VLLMModelStatus Struct", func() {
		It("should instantiate VLLMModelStatus struct without error", func() {
			// Test that the VLLMModelStatus struct exists and can be instantiated
			status := v1alpha1.VLLMModelStatus{}
			Expect(status).NotTo(BeNil())
		})
	})

	Context("VLLMModelList Struct", func() {
		It("should instantiate VLLMModelList struct without error", func() {
			// Test that the VLLMModelList struct exists and can be instantiated
			list := v1alpha1.VLLMModelList{}
			Expect(list).NotTo(BeNil())
		})
	})
})
