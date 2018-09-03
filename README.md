# SecretsEngine

SecretsEngine is a Kubernetes controller for provisioning dynamic secrets.

__This project is under active development.__

Dynamic secret provisioning allows credentials to be generated on-demand.
Without dynamic provisioning, cluster administrators typically configure a
service, such as a database, with a set of credentials and then statically
expose those credentials as a `Secret` object to Kubernetes.
Dynamic provisioning configures the service and creates a `Secret` object for
you on-demand.

The implementation of dynamic secret provisioning is based on the API Object 
`DynamicSecretConfig`. A cluster administrator can define as many
`DynamicSecretConfig` objects as needed, each describing how to configure a
service's authentication and authorization. This ensures that end users can
easily generate a secret without being exposed to credentials that have elevated
permissions.

```
apiVersion: config.secretsengine.io/v1
kind: DynamicSecretConfig
metadata:
  name: my-postgres-database
spec:
  secretName: my-postgres-database-credentials
  postgres:
    connection_uri: postgresql://localhost:5432/database
    creation:
      - CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}';
      - GRANT SELECT ON ALL TABLES IN SCHEMA public TO "{{name}}";
```

Once a `DynamicSecretConfig` has been created, end-users can create a regular
secret that references it with an annotation:

```
apiVersion: v1
kind: Secret
metadata:
  name: my-postgres-credentials
  annotations:
    secretsengine.io/dynamic-secret-config.name: my-postgres-database
```

```
kubectl create -f ./my-postgres-credentials.yaml
kubectl describe secret my-postgres-credentials
```

The SecretsEngine controller will then configure the service and populate the
secret object with newly generated credentials.

To revoke/invalidate credentials, the secret can be deleted:

```
kubectl delete secret my-postgres-credentials
```

## Supported services
- Consul
- RabbitMQ
- Databases
  - PostgreSQL
  - ... more soon.

## Key features
- Dynamic provisioning of a secret.
- Revocation only occurs when the provisioned secret is deleted.

Services that provide short-lived tokens/credentials will **NOT** be supported.
The only way credentials are invalidated is by deleting the Kubernetes secret.
This is not to suggest that long-lived credentials are better; they're not. It's
to ensure that a clear and well-defined procedure is in place for revocation so
expiration of credentials is explicit and there are no surprises.

Key rotation can easily be built on top of this foundation at a boundary that
suits the user and application, for example, at certain intervals or with every
deployment.
