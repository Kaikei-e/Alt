# Layer 01: Foundation

## 1. Responsibilities

The `01-foundation` layer is responsible for deploying the foundational, cross-cutting concerns for the entire Alt project. It establishes the necessary prerequisites for all other layers, including security, configuration, networking, and certificate management.

This layer ensures that a stable and secure base is in place before any application-specific services are deployed.

## 2. Directory Structure

```
/01-foundation/
├── charts/              # Helm charts for foundational components
│   ├── cert-manager/    # Manages TLS certificates via cert-manager
│   ├── common-config/   # Shared ConfigMaps and other resources
│   ├── common-secrets-apps/ # Base secrets for the 'alt-apps' namespace
│   ├── ca-issuer/       # Defines the self-signed CA for internal mTLS
│   └── network-policies/ # Defines baseline network security policies
└── skaffold.yaml        # Skaffold configuration for this layer
```

## 3. Deployed Components

The `skaffold.yaml` in this layer deploys the following core components via Helm:

| Helm Release          | Chart Path                  | Description                                                                                                                                                           |
| :-------------------- | :-------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cert-manager`        | `charts/cert-manager`       | Deploys `cert-manager` in the `cert-manager` namespace to automate the lifecycle of TLS certificates. Includes CRDs and necessary controllers.                           |
| `common-config`       | `charts/common-config`      | Manages shared `ConfigMap` resources, namespaces, and resource quotas within the `alt-config` namespace.                                                                |
| `common-secrets-apps` | `charts/common-secrets-apps`| Manages baseline secrets for applications in the `alt-apps` namespace, such as placeholders for database credentials and API keys.                                      |
| `ca-issuer`           | `charts/ca-issuer`          | Creates a self-signed Certificate Authority (CA) `ClusterIssuer` within the `cert-manager` namespace, used for issuing certificates for internal mTLS communication.      |
| `network-policies`    | `charts/network-policies`   | Applies fundamental network security policies, establishing a "default-deny" posture to enforce a zero-trust network model across various namespaces.                 |

## 4. Profiles

- **`dev` (Default)**: Optimized for local development (e.g., kind). It installs `cert-manager` CRDs and uses development-specific values.
- **`prod`**: Uses production-ready values for a more secure and stable configuration.