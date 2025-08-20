# Layer 04: Core Services

## 1. Responsibilities

The `04-core-services` layer is responsible for deploying the primary business logic of the Alt project. It manages the main backend API server and its supporting proxy infrastructure, which together handle client requests, interact with the data persistence layer, and control communication with external services.

## 2. Directory Structure

```
/04-core-services/
├── charts/              # Helm charts for core services
│   ├── alt-backend/     # The main Go (Echo) backend application
│   ├── envoy-proxy/     # Centralized Egress Envoy proxy
│   └── sidecar-proxy/   # A helper proxy service
└── skaffold.yaml        # Skaffold configuration for this layer
```

## 3. Build Artifacts

This layer builds two container images:

- **`kaikei/alt-backend`**: The container image for the main `alt-backend` service, built from `../../alt-backend/Dockerfile.backend`.
- **`kaikei/project-alt/alt-backend-sidecar-proxy`**: The container image for the `sidecar-proxy` service, built from `../../alt-backend/sidecar-proxy/Dockerfile`.

## 4. Deployed Components

All components in this layer are deployed into the `alt-apps` namespace.

| Helm Release      | Chart Path                | Description                                                                                                                                                                                             |
| :---------------- | :------------------------ | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `envoy-proxy`     | `charts/envoy-proxy`      | Deploys a centralized Envoy proxy that acts as a secure Egress gateway for all outbound traffic to external services. It enforces security policies, and enables consistent tracing and metrics collection. |
| `sidecar-proxy`   | `charts/sidecar-proxy`    | Deploys a helper Go proxy. Originally a sidecar, it now runs as a standalone service to handle specialized communication tasks with specific external APIs.                                                |
| `alt-backend`     | `charts/alt-backend`      | Deploys the main API server, written in Go using the Echo framework. It contains the core business logic for the application, such as RSS feed management and user settings.                               |

## 5. Architectural Strategy: Declarative Proxy Routing

The `skaffold.yaml` for this layer reveals a deliberate architectural shift towards declarative and simplified proxy routing. Previously, proxy logic may have been handled within the application, but it has been externalized:

- **Disabled Internal Proxies**: The `alt-backend` deployment explicitly sets environment variables like `PROXY_STRATEGY: "DISABLED"` and `ENVOY_PROXY_ENABLED: "false"`. This indicates that the backend application itself no longer makes dynamic decisions about how to route traffic.
- **Explicit Service URLs**: The backend is configured with the explicit URLs of the helper proxies (e.g., `SIDECAR_PROXY_BASE_URL`), making dependencies clear and routing declarative.
- **Centralized Egress**: The `envoy-proxy` serves as the single, controlled exit point for external traffic, simplifying network policies and security monitoring.

This approach, noted in comments as "Phase 7a根本解決" (Phase 7a Fundamental Solution), improves maintainability and security by making network traffic flow explicit and centrally managed rather than implicit and distributed.

## 6. Deployment Process

- **Prod-Only Profile**: This layer currently defines only a `prod` profile, indicating these services are considered stable and intended to be deployed as a consistent set.
- **Reliable Deployments**: Uses `--atomic` and `--wait` flags to ensure transactional and consistent Helm releases.
- **Post-Deployment Verification**: An `after` hook runs `kubectl get pods` to immediately verify the status of the deployed components.