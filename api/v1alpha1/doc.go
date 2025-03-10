// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// +k8s:deepcopy-gen=package
// +k8s:openapi-gen=true

//go:generate ../../hack/update-codegen.sh
//go:generate controller-gen crd:allowDangerousTypes=true paths="$PWD" output:crd:dir="../../config/crd" output:stdout

// +groupName=infrastructure.cluster.x-k8s.io
package v1alpha1 // import "github.com/gardener/cluster-api-provider-gardener/api/v1alpha1"
