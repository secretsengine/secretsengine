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
	"fmt"
	"time"

	"github.com/secretsengine/secretsengine/pkg/apis/config/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/secretsengine/provocation"
	"github.com/secretsengine/provocation/engines/password"
	"github.com/secretsengine/provocation/engines/postgresql"
	"github.com/secretsengine/provocation/engines/rabbitmq"
)

const (
	// DynamicSecretConfigNameAnnotation is the annotation used in secrets that will
	// dynamically be assigned credentials.
	DynamicSecretConfigNameAnnotation = "secretsengine.io/dynamic-secret-config.name"

	// DynamicSecretFinalizer is the finalizer added to secret resources.
	DynamicSecretFinalizer = "config.secretsengine.io/finalizer"
)

// Add creates a new Secret Controller and adds it to the Manager.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSecret{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetRecorder("secret-controller")}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New("secret-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 10,
	})
	if err != nil {
		return err
	}

	hasAnnotation := func(meta metav1.Object) bool {
		_, ok := meta.GetAnnotations()[DynamicSecretConfigNameAnnotation]
		return ok
	}

	p := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasAnnotation(e.Meta)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasAnnotation(e.Meta)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return hasAnnotation(e.MetaNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return hasAnnotation(e.Meta)
		},
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSecret{}

// ReconcileSecret reconciles a Secret object
type ReconcileSecret struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

func (r *ReconcileSecret) fetchDynamicSecretConfig(secret *corev1.Secret) (*v1alpha1.DynamicSecretConfig, error) {
	name := types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.GetAnnotations()[DynamicSecretConfigNameAnnotation],
	}

	config := &v1alpha1.DynamicSecretConfig{}
	if err := r.Get(context.TODO(), name, config); err != nil {
		r.recorder.Eventf(secret, corev1.EventTypeWarning, "DynamicSecretConfigFailed", "%v", err)
		return nil, err
	}

	return config, nil
}

func (r *ReconcileSecret) fetchDynamicSecretConfigSecret(config *v1alpha1.DynamicSecretConfig) (*corev1.Secret, error) {
	name := types.NamespacedName{
		Namespace: config.Namespace,
		Name:      config.Spec.SecretName,
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.TODO(), name, secret); err != nil {
		r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretConfigFailed", "%v", err)
		return nil, err
	}

	return secret, nil
}

func (r *ReconcileSecret) getSecretEngine(config *v1alpha1.DynamicSecretConfig) (provocation.Engine, error) {
	var engine provocation.Engine

	switch true {
	case config.Spec.Password != nil:
		options := config.Spec.Password
		engine = &password.Engine{
			Length:                options.Length,
			NumDigits:             options.NumDigits,
			NumSymbols:            options.NumSymbols,
			DisableUppercase:      options.DisableUppercase,
			AllowRepeatCharacters: options.AllowRepeatCharacters,
		}
	}

	if engine != nil {
		return engine, nil
	}

	secret, err := r.fetchDynamicSecretConfigSecret(config)
	if err != nil {
		return nil, err
	}

	switch true {
	case config.Spec.PostgreSQL != nil:
		options := config.Spec.PostgreSQL
		engine = &postgresql.Engine{
			URI:        options.URI,
			Creation:   options.Creation,
			Revocation: options.Revocation,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
		}

	case config.Spec.RabbitMQ != nil:
		options := config.Spec.RabbitMQ
		e := &rabbitmq.Engine{
			URI:      options.URI,
			Tags:     options.Tags,
			Username: string(secret.Data["username"]),
			Password: string(secret.Data["password"]),
			VHosts:   make(map[string]rabbitmq.VHost),
		}
		for vhost, permissions := range options.VHosts {
			e.VHosts[vhost] = rabbitmq.VHost(permissions)
		}
		engine = e
	}

	if engine != nil {
		return engine, nil
	}

	return nil, fmt.Errorf("no secrets engine to be configured")
}

// Reconcile reads that state of the cluster for a Secret object and makes changes based on the state read
// and what is in the Secret
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch secret
	instance := &corev1.Secret{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Fetch dynamic secret config
	config, err := r.fetchDynamicSecretConfig(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get secret engine
	engine, err := r.getSecretEngine(config)
	if err != nil {
		return reconcile.Result{}, err
	}

	// DeepCopy for modifications
	secret := DynamicSecret{instance.DeepCopy()}

	// Check to see if we're revoking
	if secret.GetDeletionTimestamp() != nil {
		// Check to see if we're the last finalizer
		finalizers := secret.GetFinalizers()
		if len(finalizers) == 0 || finalizers[0] != DynamicSecretFinalizer {
			return reconcile.Result{}, nil
		}

		// Attempt to revoke the secret
		if err = secret.Revoke(context.TODO(), engine); err != nil {
			r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretRevokeFailed", "revoking secret %v/%v failed: %v", secret.Namespace, secret.Name, err)

			return reconcile.Result{}, err
		}

		err = r.Update(context.TODO(), secret.Secret)
		if err == nil {
			r.recorder.Eventf(config, corev1.EventTypeNormal, "DynamicSecretRevoked", "revoking secret %v/%v", secret.Namespace, secret.Name)
		}

		return reconcile.Result{}, err
	}

	// Set owner
	if err := controllerutil.SetControllerReference(config, secret, r.scheme); err != nil {
		r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretSetOwnerFailed", "setting ownership on %v/%v failed: %v", secret.Namespace, secret.Name, err)

		return reconcile.Result{}, err
	}

	// Check to see if the secret is valid: already provisioned with matching
	// checksum.
	if secret.Valid() {
		return reconcile.Result{}, nil
	}

	// Attempt to revoke any previously provisioned credentials if it has
	// revocation data.
	if _, ok := secret.Annotations[DynamicSecretRevocationAnnotation]; ok {
		if err := secret.Revoke(context.TODO(), engine); err != nil {
			return reconcile.Result{}, err
		}

		err = r.Update(context.TODO(), secret.Secret)
		if err == nil {
			r.recorder.Eventf(config, corev1.EventTypeNormal, "DynamicSecretRevoked", "revoking secret %v/%v", secret.Namespace, secret.Name)
		}

		return reconcile.Result{}, err
	}

	// Attempt to provision the secret
	if err = secret.Provision(context.TODO(), engine); err != nil {
		r.recorder.Eventf(config, corev1.EventTypeNormal, "DynamicSecretProvisionFailed", "provisioning %v/%v failed: %v", secret.Namespace, secret.Name, err)

		return reconcile.Result{}, err
	}

	// Attempt to update the secret resource:
	// In the event of a failure, we'll need to rollback the provisioned
	// credentials by revoking them.
	// We loop until either the secret is updated or a rollback is successful.
	rollback := false
	backoff := wait.Backoff{
		Steps:    5,
		Duration: 10 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.1,
	}
	for {
		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			// Update
			err := r.Update(context.TODO(), secret.Secret)
			if err == nil {
				r.recorder.Eventf(config, corev1.EventTypeNormal, "DynamicSecretProvisioned", "%v/%v", secret.Namespace, secret.Name)
				return true, nil
			}
			r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretUpdateFailed", "%v/%v: %v", secret.Namespace, secret.Name, err)

			// Rollback
			err = secret.Revoke(context.TODO(), engine)
			if err == nil {
				rollback = true
				r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretRollback", "%v/%v", secret.Namespace, secret.Name)
				return true, nil
			}

			r.recorder.Eventf(config, corev1.EventTypeWarning, "DynamicSecretRollbackFailed", "%v/%v: %v", secret.Namespace, secret.Name, err)
			return false, err
		})

		if err == nil {
			break
		}
	}

	return reconcile.Result{Requeue: rollback}, nil
}
