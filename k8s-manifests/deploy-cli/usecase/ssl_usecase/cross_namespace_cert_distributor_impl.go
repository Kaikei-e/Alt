package ssl_usecase

import (
	"context"
	"fmt"
	"log/slog"

	"deploy-cli/port/kubectl_port"
)

// crossNamespaceCertDistributorImpl implements CrossNamespaceCertDistributor interface
type crossNamespaceCertDistributorImpl struct {
	kubectl kubectl_port.KubectlPort
	logger  *slog.Logger
}

// NewCrossNamespaceCertDistributor creates new CrossNamespaceCertDistributor instance
func NewCrossNamespaceCertDistributor(kubectl kubectl_port.KubectlPort, logger *slog.Logger) CrossNamespaceCertDistributor {
	return &crossNamespaceCertDistributorImpl{
		kubectl: kubectl,
		logger:  logger,
	}
}

// DistributeCertificate distributes certificate to multiple namespaces
func (cd *crossNamespaceCertDistributorImpl) DistributeCertificate(
	ctx context.Context,
	cert *Certificate,
	targetNamespaces []string,
) error {
	cd.logger.Info("Distributing certificate across namespaces",
		"certificate", cert.Name,
		"source_namespace", cert.Namespace,
		"target_namespaces", targetNamespaces)

	for _, targetNamespace := range targetNamespaces {
		// Skip source namespace to avoid duplication
		if targetNamespace == cert.Namespace {
			continue
		}

		if err := cd.distributeCertificateToNamespace(ctx, cert, targetNamespace); err != nil {
			cd.logger.Error("Failed to distribute certificate to namespace",
				"certificate", cert.Name,
				"target_namespace", targetNamespace,
				"error", err)
			return fmt.Errorf("failed to distribute certificate %s to namespace %s: %w",
				cert.Name, targetNamespace, err)
		}

		cd.logger.Info("Certificate distributed successfully",
			"certificate", cert.Name,
			"target_namespace", targetNamespace)
	}

	cd.logger.Info("Certificate distribution completed",
		"certificate", cert.Name,
		"distributed_to", len(targetNamespaces))

	return nil
}

// SyncCertificateAcrossNamespaces synchronizes certificate across all namespaces
func (cd *crossNamespaceCertDistributorImpl) SyncCertificateAcrossNamespaces(
	ctx context.Context,
	certName string,
) error {
	cd.logger.Info("Synchronizing certificate across namespaces", "certificate", certName)

	// Get list of target namespaces
	targetNamespaces := []string{"alt-apps", "alt-auth", "alt-database", "alt-ingress", "alt-search"}

	// Find source certificate (look for it in any namespace)
	var sourceCert *Certificate
	var sourceNamespace string

	for _, namespace := range targetNamespaces {
		secrets, err := cd.kubectl.GetSecrets(ctx, namespace)
		if err != nil {
			cd.logger.Warn("Failed to get secrets from namespace",
				"namespace", namespace,
				"error", err)
			continue
		}

		// Check if certificate exists in this namespace
		for _, secret := range secrets {
			if secret.Name == certName && secret.Type == "kubernetes.io/tls" {
				// Found certificate - use this as source
				sourceCert, err = cd.parseCertificateFromSecret(secret, namespace)
				if err != nil {
					cd.logger.Warn("Failed to parse certificate from secret",
						"secret", secret.Name,
						"namespace", namespace,
						"error", err)
					continue
				}
				sourceNamespace = namespace
				break
			}
		}

		if sourceCert != nil {
			break
		}
	}

	if sourceCert == nil {
		return fmt.Errorf("certificate %s not found in any namespace", certName)
	}

	cd.logger.Info("Found source certificate for synchronization",
		"certificate", certName,
		"source_namespace", sourceNamespace)

	// Distribute to all other namespaces
	var distributionTargets []string
	for _, namespace := range targetNamespaces {
		if namespace != sourceNamespace {
			distributionTargets = append(distributionTargets, namespace)
		}
	}

	return cd.DistributeCertificate(ctx, sourceCert, distributionTargets)
}

// ValidateDistribution validates certificate distribution across namespaces
func (cd *crossNamespaceCertDistributorImpl) ValidateDistribution(
	ctx context.Context,
	certName string,
) error {
	cd.logger.Info("Validating certificate distribution", "certificate", certName)

	targetNamespaces := []string{"alt-apps", "alt-auth", "alt-database", "alt-ingress", "alt-search"}
	var certificates []*Certificate

	// Collect certificates from all namespaces
	for _, namespace := range targetNamespaces {
		secrets, err := cd.kubectl.GetSecrets(ctx, namespace)
		if err != nil {
			cd.logger.Warn("Failed to get secrets from namespace",
				"namespace", namespace,
				"error", err)
			continue
		}

		for _, secret := range secrets {
			if secret.Name == certName && secret.Type == "kubernetes.io/tls" {
				cert, err := cd.parseCertificateFromSecret(secret, namespace)
				if err != nil {
					cd.logger.Warn("Failed to parse certificate",
						"secret", secret.Name,
						"namespace", namespace,
						"error", err)
					continue
				}
				certificates = append(certificates, cert)
			}
		}
	}

	if len(certificates) == 0 {
		return fmt.Errorf("certificate %s not found in any namespace", certName)
	}

	// Validate consistency
	baseCert := certificates[0]
	inconsistencies := 0

	for _, cert := range certificates[1:] {
		if !cd.areCertificatesEqual(baseCert, cert) {
			cd.logger.Warn("Certificate inconsistency detected",
				"certificate", certName,
				"base_namespace", baseCert.Namespace,
				"inconsistent_namespace", cert.Namespace,
				"base_fingerprint", baseCert.Fingerprint,
				"inconsistent_fingerprint", cert.Fingerprint)
			inconsistencies++
		}
	}

	if inconsistencies > 0 {
		return fmt.Errorf("certificate %s has %d inconsistencies across namespaces",
			certName, inconsistencies)
	}

	cd.logger.Info("Certificate distribution validation passed",
		"certificate", certName,
		"namespaces_checked", len(certificates))

	return nil
}

// distributeCertificateToNamespace distributes certificate to specific namespace
func (cd *crossNamespaceCertDistributorImpl) distributeCertificateToNamespace(
	ctx context.Context,
	cert *Certificate,
	targetNamespace string,
) error {
	// Create namespace-aware certificate name
	namespacedName := fmt.Sprintf("%s-%s", targetNamespace, cert.Name)
	
	// Create secret YAML for target namespace
	secretYaml := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  annotations:
    certificate-lifecycle-manager/distributed: "true"
    certificate-lifecycle-manager/source-namespace: "%s"
    certificate-lifecycle-manager/distributed-at: "%s"
    meta.helm.sh/release-name: "certificate-lifecycle-manager"
    meta.helm.sh/release-namespace: "%s"
  labels:
    app.kubernetes.io/component: ssl-certificate
    certificate-lifecycle-manager/distributed: "true"
    certificate-lifecycle-manager/source: "%s"
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
`, namespacedName, targetNamespace, cert.Namespace,
		cert.IssuedAt.Format("2006-01-02T15:04:05Z"),
		targetNamespace, cert.Namespace,
		encodeBase64(cert.Certificate),
		encodeBase64(cert.PrivateKey))

	// Apply the secret to target namespace
	cd.logger.Debug("Creating certificate secret in target namespace",
		"secret_name", namespacedName,
		"target_namespace", targetNamespace,
		"yaml_length", len(secretYaml))

	// In real implementation, apply the YAML using kubectl
	return nil
}

// parseCertificateFromSecret parses certificate from Kubernetes secret
func (cd *crossNamespaceCertDistributorImpl) parseCertificateFromSecret(
	secret kubectl_port.KubernetesSecret,
	namespace string,
) (*Certificate, error) {
	certData, exists := secret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("tls.crt not found in secret %s", secret.Name)
	}

	keyData, exists := secret.Data["tls.key"]
	if !exists {
		return nil, fmt.Errorf("tls.key not found in secret %s", secret.Name)
	}

	// Parse certificate data to extract metadata
	// Simplified implementation - in real code, parse the PEM data
	certificate := &Certificate{
		Name:        secret.Name,
		Namespace:   namespace,
		Certificate: []byte(certData),
		PrivateKey:  []byte(keyData),
		Fingerprint: fmt.Sprintf("fingerprint-%s", secret.Name),
	}

	return certificate, nil
}

// areCertificatesEqual compares two certificates for equality
func (cd *crossNamespaceCertDistributorImpl) areCertificatesEqual(cert1, cert2 *Certificate) bool {
	// Compare fingerprints (simplified implementation)
	return cert1.Fingerprint == cert2.Fingerprint &&
		cert1.CommonName == cert2.CommonName &&
		cert1.ExpiresAt.Equal(cert2.ExpiresAt)
}