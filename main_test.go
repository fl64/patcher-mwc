package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Suite")
}

var _ = Describe("Webhook", func() {
	var (
		server *httptest.Server
	)

	BeforeEach(func() {
		// Setup test config
		dir, _ := os.Getwd()
		testConfig := filepath.Join(dir, "testdata", "config.yaml")
		configFile = &testConfig
		loadConfig()

		mux := http.NewServeMux()

		// Register handlers with the custom mux
		mux.HandleFunc("/mutate", mutateHandler)
		mux.HandleFunc("/healthz", healthHandler)

		server = httptest.NewTLSServer(mux)
	})

	AfterEach(func() {
		server.Close()
	})

	It("should add label to Pod", func() {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
			},
		}
		objBytes, _ := json.Marshal(pod)

		ar := admissionv1.AdmissionReview{
			Request: &admissionv1.AdmissionRequest{
				UID: "test-uid",
				Kind: metav1.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Object: runtime.RawExtension{Raw: objBytes},
			},
		}

		body, _ := json.Marshal(ar)
		req, _ := http.NewRequest("POST", server.URL+"/mutate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := server.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		var result admissionv1.AdmissionReview
		json.NewDecoder(resp.Body).Decode(&result)

		Expect(result.Response.Allowed).To(BeTrue())
		Expect(result.Response.Patch).To(MatchJSON(`[
        {"op":"add","path":"/metadata/labels/example.com~1added","value":"yes"}
    ]`))
	})

	It("should handle health check", func() {
		req, _ := http.NewRequest("GET", server.URL+"/healthz", nil)

		// Используем кастомный транспорт для игнорирования TLS ошибок
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).To(Equal("OK"))
	})
})
