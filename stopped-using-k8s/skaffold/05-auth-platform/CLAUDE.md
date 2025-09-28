# Layer 05: Auth Platform

## 1. Responsibilities

The `05-auth-platform` layer is responsible for deploying the complete authentication and authorization platform for the Alt project. It manages the core components that handle user identity, sign-up/sign-in flows, session management, and access control.

## 2. Directory Structure

```
/05-auth-platform/
├── charts/              # Helm charts for authentication services
│   ├── kratos/          # Ory Kratos for identity and user management
│   └── auth-service/    # Custom Go authentication service
└── skaffold.yaml        # Skaffold configuration for this layer
```

## 3. Build Artifacts

This layer builds one container image:

- **`kaikei/alt-authservice`**: The container image for the custom `auth-service`, built from `../../auth-service/Dockerfile`.

## 4. Deployed Components

All components in this layer are deployed into the dedicated `alt-auth` namespace to isolate these critical security services.

| Helm Release   | Chart Path            | Description                                                                                                                                                               |
| :------------- | :-------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `kratos`       | `charts/kratos`       | Deploys **Ory Kratos**, an open-source identity and user management system. It handles the core authentication flows like user registration, login, and password recovery. |
| `auth-service` | `charts/auth-service` | Deploys a custom Go service that works alongside Kratos. It is responsible for project-specific authorization logic, such as issuing JWTs and validating access permissions. |

## 5. Deployment Strategy

The deployment of this critical security layer is configured to be robust and deterministic.

- **Prod-Only Profile**: Only a `prod` profile is defined, reflecting the need for a stable, consistent, and secure configuration for the authentication platform at all times.
- **Authoritative Helm Releases**: The Helm upgrade command includes the `--force` and `--reset-values` flags. This enforces a strategy where the Helm chart and its values are the absolute source of truth. Any manual configuration drift in the live cluster is forcefully overwritten during a deployment, ensuring the platform's state always matches the definition in Git.
- **Pre- and Post-Deployment Verification**: The configuration uses `before` and `after` hooks to run `helm list` and `kubectl get pods`, respectively. This provides a clear audit trail and immediate verification that the authentication components are deployed and running correctly.
- **Isolated Namespace**: All auth-related components are deployed into the `alt-auth` namespace, separating them from other application services and enforcing a clear security boundary.