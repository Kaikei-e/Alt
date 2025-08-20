# Layer 02: Infrastructure

## 1. Responsibilities

The `02-infrastructure` layer is responsible for deploying the stateful, persistent backbone of the Alt project. It manages the databases and search engines that store and serve core application data, user information, logs, and search indexes.

This layer provides the foundational data storage services upon which all other application services depend.

## 2. Directory Structure

```
/02-infrastructure/
├── charts/              # Helm charts for stateful services
│   ├── postgres/        # Main application PostgreSQL database
│   ├── kratos-postgres/ # Dedicated PostgreSQL for Ory Kratos
│   ├── auth-postgres/   # Dedicated PostgreSQL for the custom auth service
│   ├── clickhouse/      # ClickHouse for analytics and logging
│   └── meilisearch/     # Meilisearch for full-text search capabilities
└── skaffold.yaml        # Skaffold configuration for this layer
```

## 3. Build Artifacts

This layer builds a single container image:

- **`alt-atlas-migrations`**: A container image built from the `migrations-atlas` directory. It uses Atlas to apply database schema migrations and is executed as a Helm hook within the `postgres` chart deployment.

## 4. Deployed Components

This layer deploys the following stateful services via Helm:

| Helm Release      | Chart Path                | Description                                                                                                                                 |
| :---------------- | :------------------------ | :------------------------------------------------------------------------------------------------------------------------------------------ |
| `postgres`        | `charts/postgres`         | Deploys the main PostgreSQL database in the `alt-database` namespace. It includes a Helm hook to run the `alt-atlas-migrations` job.         |
| `kratos-postgres` | `charts/kratos-postgres`  | Deploys a dedicated PostgreSQL database in the `alt-auth` namespace for use by the Ory Kratos identity management service.                  |
| `auth-postgres`   | `charts/auth-postgres`    | Deploys a dedicated PostgreSQL database in the `alt-auth` namespace for the custom authentication service.                                  |
| `clickhouse`      | `charts/clickhouse`       | Deploys a ClickHouse columnar database in the `alt-analytics` namespace for storing and querying large volumes of log and analytics data.     |
| `meilisearch`     | `charts/meilisearch`      | Deploys a Meilisearch instance in the `alt-search` namespace to provide high-performance, full-text search capabilities.                   |

## 5. Deployment Strategy

Deploying stateful services requires a robust and cautious approach. This layer's `skaffold.yaml` is configured for high reliability:

- **Extended Timeout**: The `statusCheckDeadlineSeconds` is set to `9600` seconds (160 minutes) to accommodate potentially long-running tasks like data migrations and stateful service initialization.
- **Safe Helm Flags**: Uses `--atomic`, `--wait`, `--wait-for-jobs`, and `--cleanup-on-fail` flags to ensure that deployments are transactional. If a deployment fails, Helm attempts to roll back to the last successful release, preventing the cluster from being left in a broken, intermediate state.
- **Diagnostic Hooks**: Executes `kubectl` commands in `before` and `after` deployment hooks to provide real-time diagnostics. These hooks check the status of StatefulSets and Pods before and after the deployment, increasing the reliability and visibility of the process.

## 6. Profiles

- **`dev` (Default)**: For local development.
- **`staging`**: For the staging environment.
- **`prod`**: For the production environment.

Each profile primarily switches the `values.yaml` files to apply environment-specific configurations for resource allocation, replica counts, and security settings.