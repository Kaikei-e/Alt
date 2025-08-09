// OAuth2初期認証フロー実行ツール
// Inoreaderからリフレッシュトークンを取得する一回限りのツール
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	inoreaderAuthURL  = "https://www.inoreader.com/oauth2/auth"
	inoreaderTokenURL = "https://www.inoreader.com/oauth2/token"
	redirectURI       = "urn:ietf:wg:oauth:2.0:oob"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func main() {
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("INOREADER_CLIENT_ID and INOREADER_CLIENT_SECRET environment variables are required")
	}

	fmt.Println("🔐 Inoreader OAuth2 Initial Authentication Tool")
	fmt.Println("This tool will help you obtain the initial refresh token.")
	fmt.Println()

	// Step 1: Generate authorization URL
	state := fmt.Sprintf("csrf_%d", time.Now().Unix())
	authURL := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=read&state=%s",
		inoreaderAuthURL, clientID, url.QueryEscape(redirectURI), state)

	fmt.Println("📋 Step 1: Authorization")
	fmt.Printf("Please visit this URL to authorize the application:\n\n%s\n\n", authURL)
	fmt.Println("After authorization, Inoreader will display an authorization code.")
	fmt.Print("Please enter the authorization code: ")

	// Step 2: Manual input of authorization code
	var code string
	fmt.Scanln(&code)

	if code == "" {
		log.Fatal("❌ No authorization code entered")
	}

	var receivedStateValue string = state

	// Verify state parameter
	if receivedStateValue != state {
		log.Fatal("❌ CSRF check failed: state parameter mismatch")
	}

	fmt.Println("✅ Authorization code received!")
	fmt.Println()

	// Step 3: Exchange code for tokens
	fmt.Println("📋 Step 2: Token Exchange")
	tokens, err := exchangeCodeForTokens(code, clientID, clientSecret)
	if err != nil {
		log.Fatalf("❌ Failed to exchange code for tokens: %v", err)
	}

	fmt.Println("✅ Tokens obtained successfully!")
	fmt.Println()

	// Step 4: Display results
	fmt.Println("📋 Step 3: Token Information")
	fmt.Printf("Access Token: %s\n", tokens.AccessToken)
	fmt.Printf("Token Type: %s\n", tokens.TokenType)
	fmt.Printf("Expires In: %d seconds\n", tokens.ExpiresIn)
	fmt.Printf("Refresh Token: %s\n", tokens.RefreshToken)
	fmt.Printf("Scope: %s\n", tokens.Scope)
	fmt.Println()

	// Step 5: Generate kubectl commands
	fmt.Println("📋 Step 4: Kubernetes Secret Configuration")
	fmt.Println("Run the following commands to configure your Kubernetes secrets:")
	fmt.Println()
	fmt.Printf("kubectl patch secret pre-processor-sidecar-secrets -n alt-processing --type merge --patch '{\n")
	fmt.Printf("  \"data\": {\n")
	fmt.Printf("    \"INOREADER_CLIENT_ID\": \"%s\",\n", base64Encode(clientID))
	fmt.Printf("    \"INOREADER_CLIENT_SECRET\": \"%s\",\n", base64Encode(clientSecret))
	fmt.Printf("    \"INOREADER_REFRESH_TOKEN\": \"%s\",\n", base64Encode(tokens.RefreshToken))
	fmt.Printf("    \"PRE_PROCESSOR_SIDECAR_DB_PASSWORD\": \"%s\"\n", base64Encode(""))
	fmt.Printf("  }\n")
	fmt.Printf("}'\n")
	fmt.Println()

	// Step 5: Kubernetes Integration
	fmt.Println("📋 Step 5: Kubernetes Integration")
	namespace := os.Getenv("KUBERNETES_NAMESPACE")
	if namespace == "" {
		namespace = "alt-processing"
	}
	secretName := os.Getenv("OAUTH2_TOKEN_SECRET_NAME")
	if secretName == "" {
		secretName = "pre-processor-sidecar-oauth2-token"
	}

	fmt.Printf("Attempting to store token in Kubernetes Secret: %s/%s\n", namespace, secretName)

	k8sManager, err := NewKubernetesSecretManager(namespace, secretName)
	if err != nil {
		fmt.Printf("⚠️  Failed to initialize Kubernetes client: %v\n", err)
		fmt.Println("Falling back to manual kubectl commands...")

		// Fallback to displaying kubectl commands
		fmt.Println()
		fmt.Println("📋 Manual Kubernetes Secret Setup:")
		fmt.Printf("kubectl create secret generic %s -n %s \\\n", secretName, namespace)
		fmt.Printf("  --from-literal=access_token=%s \\\n", base64Encode(tokens.AccessToken))
		fmt.Printf("  --from-literal=refresh_token=%s \\\n", base64Encode(tokens.RefreshToken))
		fmt.Printf("  --from-literal=expires_at=%s\n", base64Encode(time.Now().Add(time.Duration(tokens.ExpiresIn)*time.Second).Format(time.RFC3339)))
	} else {
		// Try to create/update the secret automatically
		ctx := context.Background()
		err = k8sManager.CreateOrUpdateTokenSecret(ctx, tokens, clientID, clientSecret)
		if err != nil {
			fmt.Printf("⚠️  Failed to create/update Kubernetes secret: %v\n", err)
			fmt.Println("Please ensure proper RBAC permissions are configured.")
		} else {
			DisplayKubernetesInstructions(namespace, secretName)
		}
	}

	fmt.Println()
	fmt.Println("🎉 OAuth2 initialization completed successfully!")
	fmt.Println("Your CronJob should now be able to authenticate with Inoreader API.")
}

func exchangeCodeForTokens(code, clientID, clientSecret string) (*TokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"scope":         {""},
	}

	resp, err := http.PostForm(inoreaderTokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to make token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status: %s", resp.Status)
	}

	var tokens TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokens, nil
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
