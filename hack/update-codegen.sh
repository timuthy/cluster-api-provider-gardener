#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

CODE_GEN_DIR=$(go list -m -f '{{.Dir}}' k8s.io/code-generator)
source "${CODE_GEN_DIR}/kube_codegen.sh"

echo "Generating API groups for pkg/apis/operator"

kube::codegen::gen_helpers \
  --boilerplate "${GARDENER_HACK_DIR}/LICENSE_BOILERPLATE.txt" \
  "${PROJECT_ROOT}/api/v1alpha1"


