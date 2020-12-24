The EGW operator manages Envoy pods based on our custom resources. The
data model and operator code are scaffolded by
https://operatorframework.io/.

To generate a new custom resource definition:
```
$ operator-sdk create api --group egw --version v1 --kind EGW
```

To generate a webhook:
```
$ operator-sdk create webhook --group egw --version v1 --kind EGW --defaulting --programmatic-validation
```

## Namespaces

Each account gets their own k8s namespace, so their configuration data is separate from all other customers' data.
We need a way to find a specific customer, though, so we use the "egw" namespace as an index - links to all account namespaces are stored there.

## Naming
Because our data model is implemented using k8s custom resources, our objects' names have to live with the k8s rules for naming objects (whatever those are).

## Objects

### Account
Accounts are stored in the "egw" k8s namespace.
This lets us look up accounts using only their names.
Each account has its own namespace: "egw-accountName".
The "acme-widgets" account's namespace, for example, would be "egw-acme-widgets".

### ServiceGroup
Service Groups are stored in their owning accounts' namespaces.

### LoadBalancer
Load Balancers are stored in their owning accounts' namespaces.
