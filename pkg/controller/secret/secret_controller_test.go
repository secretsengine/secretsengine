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
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/secretsengine/secretsengine/pkg/apis/config/v1alpha1"
)

var (
	c client.Client

	expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
	secretName      = types.NamespacedName{Name: "foo", Namespace: "default"}
)

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup manager and controller.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Define secret
	instance := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
		Annotations: map[string]string{
			DynamicSecretConfigNameAnnotation: "password-secrets-engine",
		},
	}}

	// Create the secret
	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Define dynamic secret config
	config := &v1alpha1.DynamicSecretConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "password-secrets-engine",
			Namespace: "default",
		},
		Spec: v1alpha1.DynamicSecretConfigSpec{
			Password: &v1alpha1.PasswordConfig{
				Length:     10,
				NumSymbols: 5,
			},
		},
	}

	// Create the config
	err = c.Create(context.TODO(), config)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Process secret
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Check secret was updated
	checkUpdated := func() error {
		secret := &corev1.Secret{}
		err := c.Get(context.TODO(), secretName, secret)
		if err != nil {
			return err
		}

		if secret.Data == nil || len(secret.Data["password"]) != 10 || !(&DynamicSecret{secret}).Valid() {
			return fmt.Errorf("credentials were not provisioned")
		}
		return nil
	}
	g.Eventually(checkUpdated, timeout).Should(gomega.Succeed())

	// Delete secret
	g.Expect(c.Delete(context.TODO(), instance)).To(gomega.Succeed())

	// Process secret
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Check secret was deleted
	checkDeleted := func() error {
		secret := &corev1.Secret{}
		err := c.Get(context.TODO(), secretName, secret)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("secret not deleted")
	}
	g.Eventually(checkDeleted, timeout).Should(gomega.Succeed())
}
