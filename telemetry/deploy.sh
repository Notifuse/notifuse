#!/bin/bash

# Deploy script for Notifuse Telemetry Google Cloud Function
# Usage: ./deploy.sh [PROJECT_ID] [REGION]

set -e

# Default values
PROJECT_ID=${1:-"notifusev3"}
REGION=${2:-"europe-west1"}
FUNCTION_NAME="notifuse-telemetry"
ENTRY_POINT="ReceiveTelemetry"

echo "Deploying Google Cloud Function..."
echo "Project: $PROJECT_ID"
echo "Region: $REGION"
echo "Function Name: $FUNCTION_NAME"

# Deploy the function
gcloud functions deploy $FUNCTION_NAME \
  --gen2 \
  --runtime=go125 \
  --region=$REGION \
  --source=. \
  --entry-point=$ENTRY_POINT \
  --trigger-http \
  --allow-unauthenticated \
  --memory=256MB \
  --timeout=30s \
  --max-instances=10 \
  --project=$PROJECT_ID \
  --set-env-vars="GCP_PROJECT=$PROJECT_ID"

echo ""
echo "Deployment complete!"
echo ""
echo "Function URL:"
gcloud functions describe $FUNCTION_NAME --region=$REGION --project=$PROJECT_ID --format="value(serviceConfig.uri)"

echo ""
echo "To test the function, you can use:"
echo "curl -X POST \\"
echo "  \$(gcloud functions describe $FUNCTION_NAME --region=$REGION --project=$PROJECT_ID --format=\"value(serviceConfig.uri)\") \\"
echo "  -H \"Content-Type: application/json\" \\"
echo "  -d '{\"workspace_id_sha1\":\"test123\",\"contacts_count\":100}'"
