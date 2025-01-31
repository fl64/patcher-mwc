package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog"
)

var (
	codecs  = serializer.NewCodecFactory(runtime.NewScheme())
	decoder = codecs.UniversalDeserializer()
)

type Config struct {
	Mutations []Mutation `yaml:"mutations"`
}

type Mutation struct {
	Resource Resource `yaml:"resource"`
	Patches  []Patch  `yaml:"patches"`
}

type Resource struct {
	Group   string `yaml:"group"`
	Version string `yaml:"version"`
	Kind    string `yaml:"kind"`
}

type Patch struct {
	Op    string      `yaml:"op" json:"op"`
	Path  string      `yaml:"path" json:"path"`
	Value interface{} `yaml:"value" json:"value"`
}

var (
	configFile = flag.String("config", "/config/config.yaml", "Path to config file")
	port       = flag.Int("port", 8443, "Port to listen on")
	cert       = flag.String("tls-cert", "/tls/tls.crt", "TLS certificate")
	key        = flag.String("tls-key", "/tls/tls.key", "TLS private key")
	config     Config
)

func main() {
	flag.Parse()
	loadConfig()

	http.HandleFunc("/mutate", mutateHandler)
	http.HandleFunc("/healthz", healthHandler)

	klog.Infof("Starting server on :%d", *port)
	err := http.ListenAndServeTLS(
		fmt.Sprintf(":%d", *port),
		*cert,
		*key,
		nil,
	)
	if err != nil {
		klog.Fatal(err)
	}
}

func loadConfig() {
	if configFile == nil || *configFile == "" {
		panic("Config file path not specified")
	}

	data, err := os.ReadFile(*configFile)
	if err != nil {
		panic(fmt.Sprintf("Error reading config: %v", err))
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		panic(fmt.Sprintf("Error parsing config: %v", err))
	}
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func mutateHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	ar := admissionv1.AdmissionReview{}
	if _, _, err := decoder.Decode(body, nil, &ar); err != nil {
		http.Error(w, "Error decoding request", http.StatusBadRequest)
		return
	}

	if ar.Request == nil {
		http.Error(w, "Empty admission request", http.StatusBadRequest)
		return
	}
	resp := mutate(ar.Request)
	ar.Response = resp

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ar)
}

func mutate(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if req == nil {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: "Empty request",
			},
		}
	}

	gvk := schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}

	var patches []Patch
	for _, m := range config.Mutations {
		if matchResource(m.Resource, gvk) {
			patches = append(patches, m.Patches...)
		}
	}

	if len(patches) == 0 {
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true,
		}
	}

	patchBytes, _ := json.Marshal(patches)
	patchType := admissionv1.PatchTypeJSONPatch

	return &admissionv1.AdmissionResponse{
		UID:       req.UID,
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
	}
}

func matchResource(r Resource, gvk schema.GroupVersionKind) bool {
	return r.Group == gvk.Group &&
		r.Version == gvk.Version &&
		r.Kind == gvk.Kind
}
