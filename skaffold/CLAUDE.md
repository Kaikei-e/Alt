# Skaffold Orchestration Guide for the Alt Project

## 1. Overview

This document provides a comprehensive guide to the Skaffold orchestration strategy for the Alt project. The project leverages Skaffold's powerful features to manage a complex microservices architecture in a structured, repeatable, and efficient manner. The configuration is defined in the `skaffold` directory and is organized into a series of interdependent layers.

## 2. Core Principles

The entire Skaffold setup is built on four core principles:

1.  **Layered Dependencies**: The system is broken down into distinct layers, each with a specific responsibility. These layers are deployed in a strict order defined in the root `skaffold.yaml`, ensuring that foundational services are running before the components that depend on them.
2.  **Environment-Specific Configurations**: Skaffold `profiles` (`dev`, `staging`, `prod`) are used extensively to manage environment-specific settings. This allows for a single codebase while accommodating different configurations for resources, image tags, and Helm values.
3.  **Declarative Deployments**: All applications and infrastructure components are managed as Helm charts. Skaffold provides a declarative overlay to manage the deployment of these charts, injecting build-time information like image tags dynamically.
4.  **Configuration as Code**: The entire build, test, and deploy process is defined as code within the `skaffold` directory, enabling version control, peer review, and automated execution.

## 3. Layered Architecture

The root `skaffold.yaml` uses a `requires` block to define the deployment order. Each layer is a self-contained Skaffold project with its own `skaffold.yaml`.

- **`01-foundation`**: Deploys cross-cutting concerns like `cert-manager` for TLS, network policies for security, and shared configurations and secrets.
- **`02-infrastructure`**: Manages the stateful backbone of the project, including PostgreSQL databases (`postgres`, `kratos-postgres`, `auth-postgres`), ClickHouse, and MeiliSearch.
- **`04-core-services`**: Deploys the primary business logic, including the `alt-backend` (Go API) and its supporting `envoy-proxy` for controlled egress traffic.
- **`05-auth-platform`**: Manages the authentication and authorization services, consisting of Ory Kratos for identity management and a custom `auth-service`.
- **`06-application`**: Deploys the user-facing components: the `alt-frontend` (Next.js/React) and the `nginx-external` reverse proxy that acts as the system's main entry point.
- **`07-processing`**: Manages the asynchronous data processing pipeline, a collection of microservices for feed ingestion, content transformation, AI/ML tagging, and search indexing.
- **`08-operations`**: Handles operational tasks such as monitoring and database backups. *(Note: This layer's documentation is not managed by this process).*

## 4. Build Strategy

- **Artifact Definition**: Each layer's `skaffold.yaml` defines the container images to be built in its `build.artifacts` section.
- **Local Development Optimization**: `dev` profiles are configured with `push: false` and `useBuildkit: true` to enable fast, efficient local development cycles using a local Kubernetes cluster like `kind`.
- **Dynamic Tagging**: Skaffold tags images based on a defined policy (e.g., `gitCommit`) and makes these tags available for use in the deployment phase.

## 5. Deployment Strategy

- **Helm as the Engine**: Helm is the exclusive deployment tool. Skaffold orchestrates the execution of `helm install` or `helm upgrade` for each release defined in the layer-specific `skaffold.yaml` files.
- **Dynamic Value Injection**: The `setValueTemplates` feature is used to dynamically inject the correct image repository and tag into each Helm release at deploy time. This is the critical link between the build and deploy stages.

  ```yaml
  # Example from 06-application/skaffold.yaml
  deploy:
    helm:
      releases:
        - name: alt-frontend
          setValueTemplates:
            image.repository: "{{.IMAGE_REPO_kaikei_alt_frontend}}"
            image.tag: "{{.IMAGE_TAG_kaikei_alt_frontend}}" # Skaffold injects the build tag here
  ```

- **Automated Verification with Hooks**: Many layers use `before` and `after` deployment hooks to run `kubectl` and `helm` commands. These hooks perform automated checks, such as verifying that Pods are running correctly, which significantly increases deployment reliability.