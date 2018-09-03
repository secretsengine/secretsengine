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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DynamicSecretConfig is used to configure a secrets engine. It describes
// the dynamic secret that will be created and how it will be created (service,
// connection and authentication information).
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DynamicSecretConfig struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired state of a secrets engine.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec DynamicSecretConfigSpec `json:"spec,omitempty"`
}

// DynamicSecretConfigSpec defines the desired state of DynamicSecretConfig
type DynamicSecretConfigSpec struct {
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// +optional
	Consul *ConsulConfig `json:"consul,omitempty"`

	// +optional
	RabbitMQ *RabbitMQConfig `json:"rabbitmq,omitempty"`

	// +optional
	PostgreSQL *PostgreSQLConfig `json:"postgresql,omitempty"`

	// +optional
	Password *PasswordConfig `json:"password,omitempty"`
}

// ConsulConfig contains the connection and policy information used to
// provision and revoke a Consul secret.
type ConsulConfig struct {
	Address   string `json:"address"`
	TokenType string `json:"tokenType"`
	Policy    string `json:"policy"`
}

// RabbitMQConfig contains the connection, tags and vhost information used to
// provision and revoke a RabbitMQ secret.
type RabbitMQConfig struct {
	URI string `json:"uri"`

	// +optional
	Tags []string `json:"tags"`

	// +optional
	VHosts map[string]RabbitMQVhost `json:"vhosts"`
}

// RabbitMQVhost describes a RabbitMQ VHost.
type RabbitMQVhost struct {
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

// PostgreSQLConfig contains the connection infomation, and creation and
// revocation statements used to provision and revoke a PostgreSQL secret.
type PostgreSQLConfig struct {
	URI        string   `json:"uri"`
	Creation   []string `json:"creation"`
	Revocation []string `json:"revocation,omitempty"`
}

// PasswordConfig describes how to provision a random password.
type PasswordConfig struct {
	Length int `json:"length"`

	// +optional
	NumDigits int `json:"numDigits,omitempty"`

	// +optional
	NumSymbols int `json:"numSymbols,omitempty"`

	// +optional
	DisableUppercase bool `json:"disableUppercase,omitempty"`

	// +optional
	AllowRepeatCharacters bool `json:"allowRepeatCharacters,omitempty"`
}

// DynamicSecretConfigList contains a list of DynamicSecretConfig
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DynamicSecretConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicSecretConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicSecretConfig{}, &DynamicSecretConfigList{})
}
