package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/utils/colors"
	"deploy-cli/utils/logger"
)

// NewSSLCertificatesCommand creates the SSL certificates management command
func NewSSLCertificatesCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssl-certificates",
		Short: "Manage SSL certificates for services",
		Long: `Create and manage SSL certificates for Alt services.

This command suite provides comprehensive SSL certificate management including:
• Automatic SSL certificate generation for services
• Certificate validation and expiration checking
• Self-signed certificate creation for development and production
• Integration with Kubernetes secrets management

Features:
• Automatic DNS name configuration for Kubernetes services
• Certificate validation and health checking
• Expiration monitoring and renewal warnings
• Support for multiple environments (development, staging, production)

Examples:
  # Create SSL certificate for MeiliSearch
  deploy-cli ssl-certificates create meilisearch production

  # Validate existing SSL certificate
  deploy-cli ssl-certificates validate meilisearch-ssl-certs-prod alt-search

  # List all SSL certificates
  deploy-cli ssl-certificates list production

  # Check certificate expiration
  deploy-cli ssl-certificates check-expiration production

Use Cases:
• Enable HTTPS for services without manual certificate management
• Validate SSL configuration before deployment
• Monitor certificate expiration and plan renewals
• Troubleshoot SSL-related deployment issues`,
	}

	// Add subcommands
	cmd.AddCommand(newCreateSSLCommand(log))
	cmd.AddCommand(newValidateSSLCommand(log))
	cmd.AddCommand(newListSSLCommand(log))
	cmd.AddCommand(newCheckExpirationCommand(log))

	return cmd
}

// newCreateSSLCommand creates the SSL certificate creation subcommand
func newCreateSSLCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [service] [environment]",
		Short: "Create SSL certificate for a service",
		Long: `Create SSL certificate for a specific service and environment.

This command generates a self-signed SSL certificate with appropriate DNS names
and stores it as a Kubernetes secret for use by the service.

Supported Services:
• meilisearch - Full-text search engine
• postgres - Database services  
• alt-backend - Main backend service
• alt-frontend - Frontend service
• nginx - Ingress controller

Certificate Features:
• 2048-bit RSA private key
• 365-day validity period
• Automatic DNS name configuration
• Kubernetes service DNS names included
• Self-signed CA certificate included

Examples:
  # Create SSL certificate for MeiliSearch in production
  deploy-cli ssl-certificates create meilisearch production

  # Create SSL certificate for backend service in staging
  deploy-cli ssl-certificates create alt-backend staging

The generated certificate secret will be named: {service}-ssl-certs-prod`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := args[0]
			envString := args[1]

			// Parse environment
			env, err := domain.ParseEnvironment(envString)
			if err != nil {
				return fmt.Errorf("invalid environment: %w", err)
			}

			// Create SSL certificate usecase
			sslUsecase := createSSLCertificateUsecase(log)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			fmt.Printf("%s Creating SSL certificate for %s in %s environment...\n",
				colors.Blue("🔐"), serviceName, env.String())

			// Get namespace for service
			namespace := getServiceNamespace(serviceName, env)

			// Create SSL certificate based on service type
			switch serviceName {
			case "meilisearch":
				err = sslUsecase.CreateMeiliSearchSSLCertificate(ctx, namespace, env)
			case "alt-backend":
				err = sslUsecase.CreateBackendSSLCertificate(ctx, namespace, env)
			case "alt-frontend":
				err = sslUsecase.CreateFrontendSSLCertificate(ctx, namespace, env)
			case "nginx":
				err = sslUsecase.CreateNginxSSLCertificate(ctx, namespace, env)
			case "auth-service":
				err = sslUsecase.CreateAuthServiceSSLCertificate(ctx, namespace, env)
			case "kratos":
				err = sslUsecase.CreateKratosSSLCertificate(ctx, namespace, env)
			case "postgres":
				err = sslUsecase.CreatePostgresSSLCertificate(ctx, namespace, env)
			default:
				return fmt.Errorf("SSL certificate creation for service '%s' is not yet implemented", serviceName)
			}

			if err != nil {
				return fmt.Errorf("failed to create SSL certificate: %w", err)
			}

			secretName := fmt.Sprintf("%s-ssl-certs-prod", serviceName)
			fmt.Printf("%s SSL certificate created successfully\n", colors.Green("✓"))
			fmt.Printf("  Secret name: %s\n", secretName)
			fmt.Printf("  Namespace: %s\n", namespace)
			fmt.Printf("  Environment: %s\n", env.String())

			return nil
		},
	}

	return cmd
}

// newValidateSSLCommand creates the SSL certificate validation subcommand
func newValidateSSLCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [secret-name] [namespace]",
		Short: "Validate SSL certificate",
		Long: `Validate an existing SSL certificate stored in a Kubernetes secret.

This command checks:
• Certificate and private key validity
• Certificate expiration status
• DNS name configuration
• Certificate format and encoding

Validation includes:
• PEM format verification
• Certificate-key pair matching
• Expiration date checking
• Validity period verification
• Warns if certificate expires within 30 days

Examples:
  # Validate MeiliSearch SSL certificate
  deploy-cli ssl-certificates validate meilisearch-ssl-certs-prod alt-search

  # Validate backend SSL certificate
  deploy-cli ssl-certificates validate alt-backend-ssl-certs-prod alt-apps

Exit codes:
• 0: Certificate is valid
• 1: Certificate validation failed or expired`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretName := args[0]
			namespace := args[1]

			// Create SSL certificate usecase
			sslUsecase := createSSLCertificateUsecase(log)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()

			fmt.Printf("%s Validating SSL certificate %s in namespace %s...\n",
				colors.Blue("🔍"), secretName, namespace)

			err := sslUsecase.ValidateSSLCertificate(ctx, secretName, namespace)
			if err != nil {
				fmt.Printf("%s SSL certificate validation failed: %v\n", colors.Red("✗"), err)
				return err
			}

			fmt.Printf("%s SSL certificate is valid\n", colors.Green("✓"))
			return nil
		},
	}

	return cmd
}

// newListSSLCommand creates the SSL certificate listing subcommand
func newListSSLCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [environment]",
		Short: "List SSL certificates",
		Long: `List all SSL certificates managed by deploy-cli.

This command shows:
• Certificate secret names
• Associated services
• Namespaces
• Certificate types
• Management status

Only certificates created and managed by deploy-cli are shown.

Examples:
  # List SSL certificates in production
  deploy-cli ssl-certificates list production

  # List SSL certificates in staging
  deploy-cli ssl-certificates list staging`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			envString := args[0]

			// Parse environment
			env, err := domain.ParseEnvironment(envString)
			if err != nil {
				return fmt.Errorf("invalid environment: %w", err)
			}

			// Create SSL certificate usecase
			sslUsecase := createSSLCertificateUsecase(log)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()

			fmt.Printf("%s Listing SSL certificates for %s environment...\n",
				colors.Blue("📋"), env.String())

			// Get namespaces for environment
			namespaces := getEnvironmentNamespaces(env)

			var allCertificates []domain.SecretInfo
			for _, namespace := range namespaces {
				certificates, err := sslUsecase.ListSSLCertificates(ctx, namespace)
				if err != nil {
					log.Warn("Failed to list SSL certificates", "namespace", namespace, "error", err)
					continue
				}
				allCertificates = append(allCertificates, certificates...)
			}

			if len(allCertificates) == 0 {
				fmt.Printf("%s No SSL certificates found\n", colors.Yellow("ℹ"))
				return nil
			}

			fmt.Printf("\nSSL Certificates (%d found):\n", len(allCertificates))
			fmt.Println("=================================")
			for _, cert := range allCertificates {
				fmt.Printf("• %s\n", cert.Name)
				fmt.Printf("  Namespace: %s\n", cert.Namespace)
				fmt.Printf("  Service: %s\n", cert.Owner)
				fmt.Printf("  Type: %s\n", cert.Type)
				fmt.Println()
			}

			return nil
		},
	}

	return cmd
}

// newCheckExpirationCommand creates the certificate expiration checking subcommand
func newCheckExpirationCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-expiration [environment]",
		Short: "Check SSL certificate expiration",
		Long: `Check expiration status of all SSL certificates.

This command:
• Validates all SSL certificates
• Reports expiration dates
• Warns about certificates expiring soon (within 30 days)
• Identifies expired certificates

Output includes:
• Certificate names and namespaces
• Expiration dates
• Days until expiration
• Status indicators

Examples:
  # Check certificate expiration in production
  deploy-cli ssl-certificates check-expiration production

Exit codes:
• 0: All certificates are valid
• 1: Some certificates are expired or expiring soon`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			envString := args[0]

			// Parse environment
			env, err := domain.ParseEnvironment(envString)
			if err != nil {
				return fmt.Errorf("invalid environment: %w", err)
			}

			// Create SSL certificate usecase
			sslUsecase := createSSLCertificateUsecase(log)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			fmt.Printf("%s Checking SSL certificate expiration for %s environment...\n",
				colors.Blue("⏰"), env.String())

			// Get namespaces for environment
			namespaces := getEnvironmentNamespaces(env)

			var allCertificates []domain.SecretInfo
			var hasIssues bool

			for _, namespace := range namespaces {
				certificates, err := sslUsecase.ListSSLCertificates(ctx, namespace)
				if err != nil {
					log.Warn("Failed to list SSL certificates", "namespace", namespace, "error", err)
					continue
				}

				for _, cert := range certificates {
					allCertificates = append(allCertificates, cert)

					// Validate each certificate
					err := sslUsecase.ValidateSSLCertificate(ctx, cert.Name, cert.Namespace)
					if err != nil {
						fmt.Printf("%s %s/%s: %v\n", colors.Red("✗"), cert.Namespace, cert.Name, err)
						hasIssues = true
					} else {
						fmt.Printf("%s %s/%s: Valid\n", colors.Green("✓"), cert.Namespace, cert.Name)
					}
				}
			}

			if len(allCertificates) == 0 {
				fmt.Printf("%s No SSL certificates found\n", colors.Yellow("ℹ"))
				return nil
			}

			fmt.Printf("\nChecked %d SSL certificates\n", len(allCertificates))

			if hasIssues {
				fmt.Printf("%s Some certificates have issues\n", colors.Yellow("⚠"))
				return fmt.Errorf("certificate validation issues found")
			}

			fmt.Printf("%s All certificates are valid\n", colors.Green("✓"))
			return nil
		},
	}

	return cmd
}

// Helper functions

// createSSLCertificateUsecase creates SSL certificate usecase with dependencies
func createSSLCertificateUsecase(log *logger.Logger) *secret_usecase.SSLCertificateUsecase {
	// Create drivers
	kubectlDriver := kubectl_driver.NewKubectlDriver()

	// Create logger adapter
	loggerAdapter := &LoggerAdapter{logger: log}

	// Create gateways
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerAdapter)

	// Create secret usecase
	secretUsecase := secret_usecase.NewSecretUsecase(kubectlGateway, loggerAdapter)

	// Create SSL certificate usecase
	return secret_usecase.NewSSLCertificateUsecase(secretUsecase, loggerAdapter)
}

// getServiceNamespace returns the namespace for a service based on environment
func getServiceNamespace(serviceName string, env domain.Environment) string {
	switch serviceName {
	case "meilisearch":
		return "alt-search"
	case "postgres", "auth-postgres", "kratos-postgres", "clickhouse":
		return "alt-database"
	case "alt-backend", "alt-frontend", "pre-processor", "search-indexer", "tag-generator", "news-creator":
		return "alt-apps"
	case "kratos", "auth-service":
		return "alt-auth"
	case "nginx", "nginx-external":
		return "alt-ingress"
	case "monitoring":
		return "alt-observability"
	default:
		// Default to alt-apps for unknown services
		return "alt-apps"
	}
}

// getEnvironmentNamespaces returns all namespaces for an environment
func getEnvironmentNamespaces(env domain.Environment) []string {
	switch env {
	case domain.Production:
		return []string{
			"alt-apps",
			"alt-database", 
			"alt-search",
			"alt-auth",
			"alt-ingress",
			"alt-observability",
		}
	case domain.Staging:
		return []string{"alt-staging"}
	case domain.Development:
		return []string{"alt-dev"}
	default:
		return []string{"alt-apps"}
	}
}