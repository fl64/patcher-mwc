package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
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

	slog.Info("Starting server", "port", *port)
	err := http.ListenAndServeTLS(
		fmt.Sprintf(":%d", *port),
		*cert,
		*key,
		nil,
	)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

func loadConfig() {
	if configFile == nil || *configFile == "" {
		slog.Error("Config file path not specified")
		os.Exit(1)
	}

	data, err := os.ReadFile(*configFile)
	if err != nil {
		slog.Error("Error reading config", "error", err)
		os.Exit(1)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		slog.Error("Error parsing config", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func mutateHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading request", "error", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	ar := admissionv1.AdmissionReview{}
	if _, _, err := decoder.Decode(body, nil, &ar); err != nil {
		slog.Error("Error decoding request", "error", err)
		http.Error(w, "Error decoding request", http.StatusBadRequest)
		return
	}

	if ar.Request == nil {
		slog.Error("Empty admission request")
		http.Error(w, "Empty admission request", http.StatusBadRequest)
		return
	}

	resp := mutate(ar.Request)
	ar.Response = resp

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ar); err != nil {
		slog.Error("Error encoding response", "error", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

func mutate(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {

	if req == nil {
		slog.Error("Empty request")
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: "Empty request",
			},
		}
	}

	slog.Info("Got admission request", "uid", req.UID)

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
		slog.Info("No patches applied", "uid", req.UID)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true,
		}
	}

	patchBytes, _ := json.Marshal(patches)
	patchType := admissionv1.PatchTypeJSONPatch

	slog.Info("Applied patches", "uid", req.UID, "patches", patches)
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
