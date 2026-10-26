package wif

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AWSTemporaryCredentials is a minimal shape returned to callers needing AWS creds derived via WIF.
type AWSTemporaryCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// GetGCPMetadataToken fetches an OIDC token from GCP metadata server for the given audience (WIF flow).
func GetGCPMetadataToken(ctx context.Context, audience string) (string, error) {
	if os.Getenv("HITSTOR_TEST") == "true" {
		return "mock-gcp-oidc-token-for-" + audience, nil
	}
	// Standard GCP metadata endpoint for OIDC tokens used in WIF.
	u := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=" + url.QueryEscape(audience)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("wif: gcp metadata token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("wif: gcp metadata %d: %s", resp.StatusCode, string(b))
	}
	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("wif: read gcp token: %w", err)
	}
	return strings.TrimSpace(string(token)), nil
}

// ExchangeGCPTokenForAWS performs GCP OIDC -> AWS STS AssumeRoleWithWebIdentity.
func ExchangeGCPTokenForAWS(ctx context.Context, gcpToken, roleARN, roleSessionName string) (*AWSTemporaryCredentials, error) {
	if os.Getenv("HITSTOR_TEST") == "true" {
		return &AWSTemporaryCredentials{
			AccessKeyID:     "MOCKAWSACCESSKEYID",
			SecretAccessKey: "MOCKAWSSECRET",
			SessionToken:    "mock-session",
			Expiration:      time.Now().Add(15 * time.Minute),
		}, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("wif: aws config: %w", err)
	}
	// Use the token as web identity.
	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) {
		o.Credentials = credentials.NewStaticCredentialsProvider("unused", "unused", "")
	})

	out, err := stsClient.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(roleARN),
		RoleSessionName:  aws.String(roleSessionName),
		WebIdentityToken: aws.String(gcpToken),
	})
	if err != nil {
		return nil, fmt.Errorf("wif: assume role with web identity: %w", err)
	}
	if out.Credentials == nil {
		return nil, fmt.Errorf("wif: no credentials in sts response")
	}
	return &AWSTemporaryCredentials{
		AccessKeyID:     aws.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(out.Credentials.SessionToken),
		Expiration:      out.Credentials.Expiration.UTC(),
	}, nil
}

// ExchangeGCPTokenForAzure exchanges GCP OIDC token for Azure AD token via federated identity (WIF).
func ExchangeGCPTokenForAzure(ctx context.Context, gcpToken, tenantID, clientID string) (string, error) {
	if os.Getenv("HITSTOR_TEST") == "true" {
		return "mock-azure-ad-access-token", nil
	}
	// Use the GCP OIDC token as assertion for Azure federated credential.
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("grant_type", "client_credentials") // actually federated for WIF, but assertion variant:
	// For federated identity credentials the grant is "client_credentials" with client_assertion.
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", gcpToken)

	req, _ := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("wif: azure federated token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("wif: azure token %d: %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("wif: parse azure token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("wif: azure token response missing access_token")
	}
	return tok.AccessToken, nil
}
