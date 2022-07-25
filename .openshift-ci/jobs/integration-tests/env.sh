#!/usr/bin/env bash

export JOB_ID="${PROW_JOB_ID:0:8}"
export GCLOUD_INSTANCE="collector-osci-${COLLECTION_METHOD}-${VM_TYPE}-${IMAGE_FAMILY}-${JOB_ID}"
export GCP_SSH_KEY_FILE="$HOME/.ssh/GCP_SSH_KEY"
export GCLOUD_OPTIONS="--ssh-key-file=${GCP_SSH_KEY_FILE}"
export REMOTE_HOST_TYPE=gcloud
export VM_CONFIG="${VM_TYPE}.${IMAGE_FAMILY}"
export COLLECTOR_REPO="quay.io/rhacs-eng/collector"
export COLLECTOR_IMAGE="${COLLECTOR_REPO}:3.9.0"
