/*
Copyright 2018 The SecretsEngine Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secret

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/secretsengine/provocation"
	corev1 "k8s.io/api/core/v1"
)

// Annotations applied to a secret for detecting changes and revocation.
const (
	DynamicSecretChecksumAnnotation   = "secretsengine.io/dynamic-secret.checksum"
	DynamicSecretFieldsAnnotation     = "secretsengine.io/dynamic-secret.fields"
	DynamicSecretRevocationAnnotation = "secretsengine.io/dynamic-secret.revocation"
)

type DynamicSecret struct {
	*corev1.Secret
}

func (s *DynamicSecret) Valid() bool {
	fields, ok := s.Annotations[DynamicSecretFieldsAnnotation]
	if !ok {
		return false
	}

	checksum, ok := s.Annotations[DynamicSecretChecksumAnnotation]
	if !ok {
		return false
	}

	return checksum == s.checksum(strings.Fields(fields))
}

func (s *DynamicSecret) checksum(fields []string) string {
	h := sha256.New()
	for _, field := range fields {
		if data, ok := s.Data[field]; ok {
			h.Write(data)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

func (s *DynamicSecret) Revoke(ctx context.Context, engine provocation.Engine) error {
	encodedRevocation, ok := s.Annotations[DynamicSecretRevocationAnnotation]
	if !ok {
		return nil
	}

	revocation, err := base64.StdEncoding.DecodeString(encodedRevocation)
	if err != nil {
		return fmt.Errorf("error decoding revocation: %v", err)
	}

	// Remove finalizer
	s.SetFinalizers([]string{})

	// Remove annotations
	delete(s.Annotations, DynamicSecretFieldsAnnotation)
	delete(s.Annotations, DynamicSecretChecksumAnnotation)
	delete(s.Annotations, DynamicSecretRevocationAnnotation)

	return engine.Revoke(ctx, revocation)
}

func (s *DynamicSecret) Provision(ctx context.Context, engine provocation.Engine) error {
	// Provision new credentials
	revocation, credentials, err := engine.Provision(ctx, s.Namespace, s.Name)
	if err != nil {
		return err
	}

	// Update secret data
	if s.Data == nil {
		s.Data = make(map[string][]byte)
	}
	fields := make([]string, 0, len(credentials))
	for field, value := range credentials {
		s.Data[field] = value
		fields = append(fields, field)
	}

	// Update fields
	s.Annotations[DynamicSecretFieldsAnnotation] = strings.Join(fields, " ")

	// Update checksum
	s.Annotations[DynamicSecretChecksumAnnotation] = s.checksum(fields)

	// Update revocation
	s.Annotations[DynamicSecretRevocationAnnotation] = base64.StdEncoding.EncodeToString(revocation)

	// Ensure finalizer
	for _, finalizer := range s.Finalizers {
		if finalizer == DynamicSecretFinalizer {
			return nil
		}
	}
	s.Finalizers = append(s.Finalizers, DynamicSecretFinalizer)

	return nil
}
