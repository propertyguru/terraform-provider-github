#!/bin/bash
# This script compiles the provider and upload it to S3
# Usage: ./compile-and-upload-provider-to-s3.sh <version>
# Example: ./compile-and-upload-provider-to-s3.sh 4.40.0
set -o errexit
set -o nounset
set -o pipefail

# Constants
# The list of OS and architecture to build the provider for
LIST_OS=(
  "linux"
  "darwin"
)
LIST_ARCH=(
  "amd64"
  "arm64"
)
# The provider configuration
# Why we need all these variables?
# Thats because Terraform mandates the naming convention for the provider
# Read https://developer.hashicorp.com/terraform/language/providers/requirements for more information
HOSTNAME="pg-terraform-custom-providers.local"
NAMESPACE="propertyguru"
TYPE="github"
FILENAME="terraform-provider-github"
VERSION="$1"
# S3 Bucket to upload the provider to
S3_BUCKETNAME="pg-terraform-custom-providers"

echo "[INFO] Cleaning up the previous build if any"
rm -rf "${HOSTNAME}"

echo "[INFO] Compiling provider and upload it to S3"
for os in "${LIST_OS[@]}"; do
  for arch in "${LIST_ARCH[@]}"; do
      path="${HOSTNAME}/${NAMESPACE}/${TYPE}/${VERSION}/${os}_${arch}/${FILENAME}"
      echo "[INFO] Building version ${VERSION} for ${os}_${arch}, and store it to ${path}"
      GOOS="${os}" GOARCH="${arch}" go build -gcflags="all=-N -l" -o "${path}"
      checksum=$(md5sum "${path}" | awk '{print $1}')
      echo "[INFO] Built successful. Uploading to S3 with checksum ${checksum}"
      aws s3 cp \
        --quiet \
        --metadata "checksum=${checksum}" \
        "${path}" "s3://${S3_BUCKETNAME}/${path}"
      echo
  done
done
