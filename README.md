# Patcher Mutation Webhook Controller

This repository contains a simple Kubernetes Admission Controller written in Go. The purpose of this code is strictly for testing and debugging purposes. It is not intended for production use.

## Overview

The admission controller listens for incoming admission requests and applies mutations to Kubernetes resources based on a provided configuration file. The configuration file specifies which resources should be mutated and the patches to be applied.

## Features

- Admission Review Handling: The controller handles Kubernetes AdmissionReview requests and responds with appropriate mutations.

- YAML Configuration: Mutations are defined in a YAML configuration file, allowing for easy customization.

- TLS Support: The server runs with TLS encryption, requiring a certificate and private key.

- Health Check: Includes a /healthz endpoint for basic health checks.

## Configuration

The configuration file (config.yaml) specifies the resources to be mutated and the patches to apply. Example configuration:

```yaml
mutations:

- resource:
  group: "apps"
  version: "v1"
  kind: "Deployment"
  patches:
  - op: "add"
    path: "/metadata/labels/test"
    value: "true"
```

## Disclaimer

This code is provided as-is and is not intended for production use. It is meant for testing and debugging purposes only. Use at your own risk.
