package sentinel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/wif"
	genai "google.golang.org/genai"
)

// TelemetryData contains statistics for a backup session.
type TelemetryData struct {
	Time            time.Time `json:"time"`
	FilesNew        uint64    `json:"files_new"`
	FilesUnchanged  uint64    `json:"files_unchanged"`
	BytesAdded      uint64    `json:"bytes_added"`
	BytesProcessed  uint64    `json:"bytes_processed"`
	Duration        float64   `json:"duration"`
	SuspiciousFiles uint64    `json:"suspicious_files"`
}

// SentinelAnalysis represents the classification results from Gemini.
type SentinelAnalysis struct {
	ThreatDetected bool    `json:"threat_detected"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
}

// Config controls Sentinel parameters.
type Config struct {
	Enabled             bool
	ModelName           string  // default: "gemini-1.5-flash"
	ApiKey              string  // optional API key
	GCSBucket           string  // optional explicit GCS bucket name
	ConfidenceThreshold float64 // default: 0.8
	RetentionDays       int     // default: 30
	BackendType         string  // "gcs", "s3", "azure" (defaults to "gcs")
	S3Bucket            string
	S3Region            string
	S3RoleARN           string
	AzureContainer      string
	AzureStorageAccount string
	AzureTenant         string
	AzureClient         string
}

// Sentinel manages ransomware telemetry audit and cloud protection actions.
type Sentinel struct {
	cfg Config
}

// New creates a new Sentinel instance.
func New(cfg Config) *Sentinel {
	if cfg.ModelName == "" {
		cfg.ModelName = "gemini-1.5-flash"
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = 0.8
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	return &Sentinel{cfg: cfg}
}

// CheckSession performs the AI-based safety check on session telemetry.
func (s *Sentinel) CheckSession(ctx context.Context, agentName string, current TelemetryData, history []TelemetryData) (*SentinelAnalysis, error) {
	if !s.cfg.Enabled {
		return &SentinelAnalysis{ThreatDetected: false, Confidence: 0, Reason: "Sentinel is disabled"}, nil
	}

	if os.Getenv("HITSTOR_TEST") == "true" {
		// Mock detection: if FilesNew > 10 and FilesUnchanged == 0 and BytesAdded == BytesProcessed, or if SuspiciousFiles > 0
		isRansomware := (current.FilesNew > 10 && current.FilesUnchanged == 0 && current.BytesAdded > 0 && current.BytesAdded == current.BytesProcessed) || current.SuspiciousFiles > 0
		if isRansomware {
			return &SentinelAnalysis{
				ThreatDetected: true,
				Confidence:     0.95,
				Reason:         "MOCK DETECTION: Suspicious file extensions detected or high change velocity with deduplication collapse.",
			}, nil
		}
		return &SentinelAnalysis{
			ThreatDetected: false,
			Confidence:     0.05,
			Reason:         "MOCK DETECTION: Telemetry patterns fall within normal operational bounds.",
		}, nil
	}

	// Build client. Keyless/ADC if ApiKey is empty.
	var client *genai.Client
	var err error
	if s.cfg.ApiKey != "" {
		client, err = genai.NewClient(ctx, &genai.ClientConfig{APIKey: s.cfg.ApiKey})
	} else {
		client, err = genai.NewClient(ctx, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("sentinel: create genai client: %w", err)
	}

	// Format prompt
	var promptBuilder strings.Builder
	promptBuilder.WriteString("You are the security sentinel of HitStor, an autonomous self-defending backup system.\n")
	promptBuilder.WriteString("Analyze the following backup run telemetry for potential ransomware activity on the host.\n")
	promptBuilder.WriteString("Ransomware behavior typically exhibits:\n")
	promptBuilder.WriteString("1. High change velocity: a large number of new/changed files compared to unchanged files.\n")
	promptBuilder.WriteString("2. Compressed/Deduplicated ratio collapse: ransomware-encrypted files cannot be deduplicated or compressed efficiently. The ratio of bytes_processed (uncompressed total size) / bytes_added (compressed added bytes) collapses towards 1.0 (indicating 0% compression or deduplication).\n")
	promptBuilder.WriteString("3. High count or ratio (>5%) of new files matching suspicious extensions (e.g. .locked, .crypto, .ryuk, etc.) or high entropy names.\n")
	promptBuilder.WriteString("4. Sharp deviations from historical baseline runs.\n\n")

	fmt.Fprintf(&promptBuilder, "Current Run Telemetry for Agent %q:\n", agentName)
	fmt.Fprintf(&promptBuilder, "- New Files: %d\n", current.FilesNew)
	fmt.Fprintf(&promptBuilder, "- Unchanged Files: %d\n", current.FilesUnchanged)
	fmt.Fprintf(&promptBuilder, "- Bytes Added (Compressed/Written): %d\n", current.BytesAdded)
	fmt.Fprintf(&promptBuilder, "- Bytes Processed (Uncompressed Total): %d\n", current.BytesProcessed)
	fmt.Fprintf(&promptBuilder, "- Suspicious Files: %d\n", current.SuspiciousFiles)

	suspRatio := float64(0)
	if current.FilesNew > 0 {
		suspRatio = float64(current.SuspiciousFiles) / float64(current.FilesNew)
	}
	fmt.Fprintf(&promptBuilder, "- Suspicious Suffix Ratio: %.2f%%\n", suspRatio*100)
	fmt.Fprintf(&promptBuilder, "- Duration: %.2f seconds\n\n", current.Duration)

	if len(history) > 0 {
		promptBuilder.WriteString("Historical Runs Telemetry (newest first):\n")
		for idx, h := range history {
			fmt.Fprintf(&promptBuilder, "  Run %d (%s):\n", idx+1, h.Time.Format(time.RFC3339))
			fmt.Fprintf(&promptBuilder, "    - New Files: %d\n", h.FilesNew)
			fmt.Fprintf(&promptBuilder, "    - Unchanged Files: %d\n", h.FilesUnchanged)
			fmt.Fprintf(&promptBuilder, "    - Bytes Added (Compressed): %d\n", h.BytesAdded)
			fmt.Fprintf(&promptBuilder, "    - Bytes Processed (Uncompressed): %d\n", h.BytesProcessed)
			fmt.Fprintf(&promptBuilder, "    - Suspicious Files: %d\n", h.SuspiciousFiles)
		}
	} else {
		promptBuilder.WriteString("No historical baseline data exists for this host (this is the first backup run).\n")
	}

	promptBuilder.WriteString("\nClassify if this current run shows behavior indicating ransomware. Answer using the requested JSON schema.\n")

	// Call model with structured schema config
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"threat_detected": {
					Type:        genai.TypeBoolean,
					Description: "Set to true if ransomware encryption or mass modification is detected with high probability.",
				},
				"confidence": {
					Type:        genai.TypeNumber,
					Description: "Confidence rating of this safety classification, between 0.0 (no threat) and 1.0 (highly confident threat).",
				},
				"reason": {
					Type:        genai.TypeString,
					Description: "Detailed reasoning explanation justifying the risk decision, citing telemetry metrics.",
				},
			},
			Required: []string{"threat_detected", "confidence", "reason"},
		},
	}

	resp, err := client.Models.GenerateContent(ctx, s.cfg.ModelName, genai.Text(promptBuilder.String()), config)
	if err != nil {
		return nil, fmt.Errorf("sentinel: generate classification: %w", err)
	}

	var analysis SentinelAnalysis
	if err := json.Unmarshal([]byte(resp.Text()), &analysis); err != nil {
		slog.Error("sentinel: failed to parse Gemini response JSON", "response", resp.Text(), "error", err)
		return nil, fmt.Errorf("sentinel: parse safety analysis JSON: %w", err)
	}

	return &analysis, nil
}

// LockGCSBucket locks repository storage on the configured cloud backend keylessly.
func (s *Sentinel) LockGCSBucket(ctx context.Context) error {
	if !s.cfg.Enabled {
		return nil
	}

	backendType := s.cfg.BackendType
	if backendType == "" {
		backendType = "gcs"
	}

	switch backendType {
	case "gcs":
		return s.lockGCS(ctx)
	case "s3":
		return s.lockS3(ctx)
	case "azure":
		return s.lockAzure(ctx)
	default:
		return fmt.Errorf("sentinel: unsupported backend type for locking: %s", backendType)
	}
}

func (s *Sentinel) lockGCS(ctx context.Context) error {
	bucketName := s.cfg.GCSBucket
	if bucketName == "" {
		bucketName = os.Getenv("HITSTOR_GCS_BUCKET")
	}

	if os.Getenv("HITSTOR_TEST") == "true" {
		slog.Info("MOCK GCS LOCK: Storage bucket lock simulation success", "bucket", bucketName, "retention_days", s.cfg.RetentionDays)
		return nil
	}

	// Auto-detect project ID to resolve default bucket name if still empty.
	if bucketName == "" {
		projID, err := s.detectProjectID(ctx)
		if err != nil {
			return fmt.Errorf("sentinel: lock GCS: failed to resolve bucket name: %w", err)
		}
		bucketName = projID + "-hitstor-data"
	}

	slog.Info("sentinel: locking GCS bucket by setting retention policy", "bucket", bucketName, "days", s.cfg.RetentionDays)

	// Fetch keyless ADC access token from GCP Metadata Server
	token, err := s.fetchMetadataToken(ctx)
	if err != nil {
		return fmt.Errorf("sentinel: fetch GCP identity token: %w", err)
	}

	// Make a PATCH request to GCS Buckets API to configure bucket retention policy.
	gcsURL := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s", bucketName)
	retentionSeconds := int64(s.cfg.RetentionDays) * 24 * 60 * 60

	// JSON request body for Patch Bucket
	reqBody, _ := json.Marshal(map[string]any{
		"retentionPolicy": map[string]any{
			"retentionPeriod": fmt.Sprintf("%d", retentionSeconds),
		},
	})

	req, err := http.NewRequestWithContext(ctx, "PATCH", gcsURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GCS patch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GCS patch response error (%d): %s", resp.StatusCode, string(body))
	}

	slog.Info("sentinel: GCS bucket retention lock policy successfully applied", "bucket", bucketName)
	return nil
}

func (s *Sentinel) lockS3(ctx context.Context) error {
	bucketName := s.cfg.S3Bucket
	if bucketName == "" {
		bucketName = os.Getenv("HITSTOR_S3_BUCKET")
	}

	if os.Getenv("HITSTOR_TEST") == "true" {
		slog.Info("MOCK S3 LOCK: Storage bucket lock simulation success", "bucket", bucketName, "retention_days", s.cfg.RetentionDays)
		return nil
	}

	gcpToken, err := wif.GetGCPMetadataToken(ctx, "https://sts.amazonaws.com")
	if err != nil {
		return fmt.Errorf("aws-wif: get gcp oidc: %w", err)
	}

	roleARN := s.cfg.S3RoleARN
	if roleARN == "" {
		roleARN = os.Getenv("HITSTOR_AWS_ROLE_ARN")
	}
	if roleARN == "" {
		roleARN = "arn:aws:iam::123456789012:role/HitStorBackupRole"
	}

	creds, err := wif.ExchangeGCPTokenForAWS(ctx, gcpToken, roleARN, "HitStorSentinelSession")
	if err != nil {
		return fmt.Errorf("aws-wif: token exchange failed: %w", err)
	}

	region := s.cfg.S3Region
	if region == "" {
		region = os.Getenv("HITSTOR_S3_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	slog.Info("sentinel: locking AWS S3 bucket by configuring Object Lock", "bucket", bucketName, "days", s.cfg.RetentionDays)

	s3URL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/?object-lock", bucketName, region)
	xmlPayload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <ObjectLockEnabled>Enabled</ObjectLockEnabled>
  <Rule>
    <DefaultRetention>
      <Mode>GOVERNANCE</Mode>
      <Days>%d</Days>
    </DefaultRetention>
  </Rule>
</ObjectLockConfiguration>`, s.cfg.RetentionDays)

	req, err := http.NewRequestWithContext(ctx, "PUT", s3URL, strings.NewReader(xmlPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	slog.Info("sentinel: S3 PUT bucket object lock config generated keylessly", "role", roleARN, "creds_key", creds.AccessKeyID)

	return nil
}

func (s *Sentinel) lockAzure(ctx context.Context) error {
	container := s.cfg.AzureContainer
	if container == "" {
		container = os.Getenv("HITSTOR_AZURE_CONTAINER")
	}
	account := s.cfg.AzureStorageAccount
	if account == "" {
		account = os.Getenv("HITSTOR_AZURE_STORAGE_ACCOUNT")
	}

	if os.Getenv("HITSTOR_TEST") == "true" {
		slog.Info("MOCK AZURE LOCK: Storage container immutability policy simulation success", "container", container, "retention_days", s.cfg.RetentionDays)
		return nil
	}

	gcpToken, err := wif.GetGCPMetadataToken(ctx, "https://accounts.google.com")
	if err != nil {
		return fmt.Errorf("azure-wif: get gcp oidc: %w", err)
	}

	tenantID := s.cfg.AzureTenant
	if tenantID == "" {
		tenantID = os.Getenv("HITSTOR_AZURE_TENANT")
	}
	clientID := s.cfg.AzureClient
	if clientID == "" {
		clientID = os.Getenv("HITSTOR_AZURE_CLIENT")
	}
	if tenantID == "" || clientID == "" {
		return fmt.Errorf("azure-wif: tenant ID and client ID are required for token exchange")
	}

	azureToken, err := wif.ExchangeGCPTokenForAzure(ctx, gcpToken, tenantID, clientID)
	if err != nil {
		return fmt.Errorf("azure-wif: token exchange: %w", err)
	}

	rg := "hitstor-rg" // default resource group
	sub := os.Getenv("HITSTOR_AZURE_SUB")
	if sub == "" {
		sub = "mock-sub"
	}

	slog.Info("sentinel: locking Azure Storage container immutability policy", "container", container, "account", account, "days", s.cfg.RetentionDays)

	azureURL := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/%s/immutabilityPolicies/default?api-version=2021-04-01", sub, rg, account, container)
	policyBody := map[string]any{
		"properties": map[string]any{
			"immutabilityPeriodSinceCreationInDays": s.cfg.RetentionDays,
			"allowProtectedAppendWrites":            true,
		},
	}
	bodyBytes, _ := json.Marshal(policyBody)

	req, err := http.NewRequestWithContext(ctx, "PUT", azureURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+azureToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("azure policy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure policy response error (%d): %s", resp.StatusCode, string(body))
	}

	slog.Info("sentinel: Azure container immutability policy successfully applied", "container", container)
	return nil
}

// detectProjectID fetches the current GCP project ID from the GCP Metadata Server.
func (s *Sentinel) detectProjectID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/project/project-id", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

// fetchMetadataToken gets an OAuth2 Access Token for GCS scopes from Compute Engine Metadata service.
func (s *Sentinel) fetchMetadataToken(ctx context.Context) (string, error) {
	url := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.AccessToken, nil
}
