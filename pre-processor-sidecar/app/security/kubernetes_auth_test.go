package security

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateKubernetesServiceAccountToken_RejectsWhenPublicKeyUnavailable(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
		// publicKey intentionally nil
	}

	claims := &ServiceAccountClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "system:serviceaccount:alt-processing:pre-processor-admin",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		Kubernetes: KubernetesClaims{
			Namespace: "alt-processing",
			ServiceAccount: ServiceAccountReference{
				Name: "pre-processor-admin",
				UID:  "uid-1",
			},
		},
	}

	// HS256 token simulates untrusted/signed-with-wrong-key input.
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("not-used"))
	require.NoError(t, err)

	info, validateErr := auth.ValidateKubernetesServiceAccountToken(tokenString)
	require.Error(t, validateErr)
	assert.Nil(t, info)
}

func TestHasAdminPermissions_DenyImplicitNamespaceGrant(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
	}

	info := &ServiceAccountInfo{
		Subject:   "system:serviceaccount:alt-processing:reader",
		Namespace: "alt-processing",
		Name:      "reader",
	}

	assert.False(t, auth.HasAdminPermissions(info))
}

func TestHasAdminPermissions_DenyDefaultServiceAccount(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
	}

	info := &ServiceAccountInfo{
		Subject:   "system:serviceaccount:alt-processing:default",
		Namespace: "alt-processing",
		Name:      "default",
	}

	assert.False(t, auth.HasAdminPermissions(info))
}

func TestHasAdminPermissions_DenyAdminSAFromOtherNamespace(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
	}

	info := &ServiceAccountInfo{
		Subject:   "system:serviceaccount:other:pre-processor-admin",
		Namespace: "other",
		Name:      "pre-processor-admin",
	}

	assert.False(t, auth.HasAdminPermissions(info))
}

func TestHasAdminPermissions_AllowExplicitAdminServiceAccount(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
	}

	info := &ServiceAccountInfo{
		Subject:   "system:serviceaccount:alt-processing:pre-processor-admin",
		Namespace: "alt-processing",
		Name:      "pre-processor-admin",
	}

	assert.True(t, auth.HasAdminPermissions(info))
}

func TestHasAdminPermissions_RespectsEnvOverrideAllowlist(t *testing.T) {
	t.Setenv("PRE_PROCESSOR_ADMIN_SERVICE_ACCOUNTS", "custom-admin-sa")
	t.Setenv("PRE_PROCESSOR_ADMIN_SUBJECTS", "system:serviceaccount:alt-processing:custom-admin-sa")

	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "alt-processing",
	}

	info := &ServiceAccountInfo{
		Subject:   "system:serviceaccount:alt-processing:custom-admin-sa",
		Namespace: "alt-processing",
		Name:      "custom-admin-sa",
	}

	assert.True(t, auth.HasAdminPermissions(info))
}

func TestIsDevelopmentEnvironment_StillWorksForExistingBehavior(t *testing.T) {
	auth := &KubernetesAuthenticator{
		logger:    slog.Default(),
		namespace: "prod",
	}

	os.Setenv("ENVIRONMENT", "development")
	defer os.Unsetenv("ENVIRONMENT")

	assert.True(t, auth.isDevelopmentEnvironment())
}
