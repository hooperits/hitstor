#!/usr/bin/env bash
#
# Deploy HITStor Sentinel to Google Cloud Run with GCS Fuse Volume Mounts
#

set -euo pipefail

# Default Configuration
REGION="${GCP_REGION:-us-central1}"
SERVICE_NAME="hitstor-sentinel"
REPO_NAME="hitstor-images"
DOCKER_TAG="latest"

echo "=================================================================="
echo "    HITStor Sentinel: GCP Cloud Run Deployment Automation         "
echo "=================================================================="

# Check for gcloud installation
if ! command -v gcloud &> /dev/null; then
    echo "Error: Google Cloud SDK (gcloud) is not installed."
    echo "Please install it from https://cloud.google.com/sdk/docs/install"
    exit 1
fi

# Get current project ID
PROJECT_ID=$(gcloud config get-value project 2>/dev/null)
if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "(unset)" ]; then
    echo "Error: No GCP Project ID is set in gcloud config."
    echo "Please set one using: gcloud config set project <PROJECT_ID>"
    exit 1
fi

echo "Deploying to Project: ${PROJECT_ID}"
echo "Region: ${REGION}"

# Read password for the backup repository
read -s -p "Enter Repository Master Password (HITSTOR_PASSWORD): " MASTER_PASSWORD
echo ""
if [ -z "$MASTER_PASSWORD" ]; then
    echo "Error: Password cannot be empty."
    exit 1
fi

# Define Bucket Name (must be globally unique, using project ID)
BUCKET_NAME="${PROJECT_ID}-hitstor-data"

echo "------------------------------------------------------------------"
echo "Step 1: Enabling Required GCP Services..."
echo "------------------------------------------------------------------"
gcloud services enable \
    run.googleapis.com \
    artifactregistry.googleapis.com \
    cloudbuild.googleapis.com \
    storage.googleapis.com

echo "------------------------------------------------------------------"
echo "Step 2: Creating Cloud Storage Bucket (for persistent metadata & repo)..."
echo "------------------------------------------------------------------"
if ! gcloud storage buckets describe "gs://${BUCKET_NAME}" &>/dev/null; then
    echo "Creating bucket gs://${BUCKET_NAME} in region ${REGION}..."
    gcloud storage buckets create "gs://${BUCKET_NAME}" \
        --location="${REGION}" \
        --uniform-bucket-level-access
else
    echo "Bucket gs://${BUCKET_NAME} already exists."
fi

echo "------------------------------------------------------------------"
echo "Step 3: Preparing Artifact Registry for Docker image..."
echo "------------------------------------------------------------------"
IMAGE_PATH="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/${SERVICE_NAME}:${DOCKER_TAG}"
if ! gcloud artifacts repositories describe "${REPO_NAME}" --location="${REGION}" &>/dev/null; then
    echo "Creating Artifact Registry repository ${REPO_NAME}..."
    gcloud artifacts repositories create "${REPO_NAME}" \
        --repository-format=docker \
        --location="${REGION}" \
        --description="Docker repository for HitStor Sentinel"
else
    echo "Artifact Registry repository ${REPO_NAME} already exists."
fi

echo "------------------------------------------------------------------"
echo "Step 4: Building and pushing Docker Image using Cloud Build..."
echo "------------------------------------------------------------------"
# Cloud Build builds the multi-binary Docker container and pushes it to the Registry
gcloud builds submit --tag "${IMAGE_PATH}" .

echo "------------------------------------------------------------------"
echo "Step 5: Deploying to Google Cloud Run (with GCS Volume Mount)..."
echo "------------------------------------------------------------------"
# We deploy to Cloud Run, attaching the GCS bucket as a volume mount to /var/lib/hitstor
# This persists the SQLite cache, agent registry, and backup packs.
gcloud beta run deploy "${SERVICE_NAME}" \
    --image="${IMAGE_PATH}" \
    --region="${REGION}" \
    --allow-unauthenticated \
    --port=8421 \
    --add-volume="name=hitstor-storage,type=cloud-storage,bucket=${BUCKET_NAME}" \
    --add-volume-mount="volume=hitstor-storage,mount-path=/var/lib/hitstor" \
    --set-env-vars="HITSTOR_REPO=/var/lib/hitstor/repo,HITSTOR_PASSWORD=${MASTER_PASSWORD},HITSTOR_PORT=8420,HITSTOR_DASHBOARD_PORT=8421" \
    --memory=1Gi \
    --cpu=1

echo "=================================================================="
echo "Deployment Complete!"
echo "HITStor Sentinel Dashboard is available at the Cloud Run Service URL."
echo "Backup agents should register using the HTTPS Service URL on port 443."
echo "=================================================================="
