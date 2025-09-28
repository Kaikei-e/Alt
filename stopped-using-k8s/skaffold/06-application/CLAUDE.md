# Layer 06: Application

## 1. Responsibilities

The `06-application` layer is responsible for deploying the user-facing components of the Alt project. This includes the frontend user interface and the external entry point that routes incoming traffic to the appropriate internal services.

## 2. Directory Structure

```
/06-application/
├── charts/              # Helm charts for application components
│   ├── alt-frontend/    # The Next.js/React frontend application
│   └── nginx-external/  # NGINX reverse proxy for external traffic
└── skaffold.yaml        # Skaffold configuration for this layer
```

## 3. Build Artifacts

This layer builds the frontend application container image. The image name varies by profile:

- **`dev` profile**: `alt-frontend`
- **`prod` profile**: `kaikei/alt-frontend`

The image is built from `../../alt-frontend/Dockerfile.frontend`.

## 4. Deployed Components

This layer deploys the user interface and the primary ingress point, separating them by namespace to maintain a clean architecture.

| Helm Release     | Chart Path                | Namespace     | Description                                                                                                                                                                 |
| :--------------- | :------------------------ | :------------ | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alt-frontend`   | `charts/alt-frontend`     | `alt-apps`    | Deploys the Next.js/React single-page application (SPA). This is the primary interface that users interact with in their browsers.                                                |
| `nginx-external` | `charts/nginx-external`   | `alt-ingress` | Deploys an NGINX reverse proxy that serves as the external entry point for all HTTP/S traffic. It routes requests to the correct internal services, starting with `alt-frontend`. |

## 5. Deployment Strategy

- **Separation of Concerns**: The deployment strategy clearly separates the user-facing application from the ingress infrastructure. The `alt-frontend` lives with other applications in `alt-apps`, while the `nginx-external` proxy resides in the dedicated `alt-ingress` namespace. This isolates network entry points from application logic, improving security and maintainability.
- **Profile-Specific Builds**: The `skaffold.yaml` uses different image names and build arguments for `dev` and `prod` profiles, allowing for environment-specific configurations (e.g., pointing to different backend URLs) to be baked into the frontend image at build time.
- **Atomic Deployments**: The use of `--atomic` and `--wait` flags for Helm ensures that deployments are transactional. A failed deployment will be rolled back, preventing the application from being left in a broken state.

## 6. Profiles

- **`dev` (Default)**: Optimized for local development. It uses a local image name (`alt-frontend`) and sets `image.pullPolicy: "Never"` to ensure the locally built image is used.
- **`prod`**: Configured for production. It uses the production image name (`kaikei/alt-frontend`) and applies production-specific values.