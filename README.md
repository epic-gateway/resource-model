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

## ID/Key Allocation

Numeric ID's (like Tunnel ID) and keys (like ServiceID and GroupID)
are allocated from fields in their parent CR Status. Tunnel IDs are
unique across the whole system so they're allocated from the egw
singleton object. You can examine the current value by dumping the contents of the object:

```
$ kubectl get -n egw egws.egw.acnodal.io egw -o yaml
apiVersion: egw.acnodal.io/v1
kind: EGW
metadata:
  ... snip ...
  name: egw
  namespace: egw
  resourceVersion: "22829"
  uid: e6a50ecf-1829-4e47-8537-abfd9b6a9146
spec: {}
status:
  current-account-gue-key: 1
  current-tunnel-id: 4
```

The ```current-account-gue-key``` holds the most recent value that we
assigned to an Account object.

The ```current-tunnel-id``` field holds the most recent value that we
assigned to a tunnel ID (within a Service object).

When we assign an account GUE key we add it to the spec.gue-key field
of the Account custom resource:

```
$ kubectl get -n egw accounts.egw.acnodal.io sample -o yaml
apiVersion: egw.acnodal.io/v1
kind: Account
metadata:
  ... snip ...
  name: sample
  namespace: egw
  resourceVersion: "22769"
  uid: 2c422e97-9d3f-4181-9b07-59484ce09566
spec:
  gue-key: 1
status:
  current-service-gue-key: 3
```

There's also a ```current-service-gue-key``` field within the status
which is the source of GUE keys for the services within that account.

```
$ kubectl get -n egw-sample loadbalancers.egw.acnodal.io sample-acnodal -o yaml
apiVersion: egw.acnodal.io/v1
kind: LoadBalancer
metadata:
  ... snip ...
  name: sample-acnodal
  namespace: egw-sample
  resourceVersion: "2584"
  uid: 1a8522b9-70a4-4a9c-bdd7-ba912a61170a
spec:
  gue-key: 65537
  public-address: 192.168.77.2
  public-ports:
  - nodePort: 30390
    port: 8888
    protocol: TCP
    targetPort: 8080
  service-group: sample
status:
  gue-tunnel-endpoints:
    192.168.1.16:
      egw-address: 192.168.1.40
      egw-port:
        appProtocol: gue
        port: 6080
        protocol: UDP
      tunnel-id: 2
    192.168.1.25:
      egw-address: 192.168.1.40
      egw-port:
        appProtocol: gue
        port: 6080
        protocol: UDP
      tunnel-id: 1
  proxy-ifindex: 16
  proxy-ifname: veth67060f1f
```

The gue-key in the spec is the 32-bit value that's the combination of
the account key in the upper 16 bits and the service key in the
lower. In this case 65537 is 0x00010001 which indicates an account key
of 1 and a service key of 1.

Each tunnel in the status has a tunnel ID that was allocated from the
EGW configuration singleton.

We also cache some values in endpoint objects, mostly so we can use
them to clean up when the parent load balancer is deleted.

```
$ kubectl get -n egw-sample endpoints.egw.acnodal.io d2e08096-a3e4-4d1f-8555-d1641c7fa9ed -o yaml
apiVersion: egw.acnodal.io/v1
kind: Endpoint
metadata:
  ... snip ...
  name: d2e08096-a3e4-4d1f-8555-d1641c7fa9ed
  namespace: egw-sample
  resourceVersion: "22850"
  uid: 0c853955-2b5b-4f20-b67f-e56b9387766b
spec:
  address: 10.244.1.5
  load-balancer: sample2-acnodal
  node-address: 192.168.1.25
  port:
    name: second
    port: 8080
    protocol: TCP
status:
  gue-key: 65539
  proxy-ifindex: 20
  tunnel-id: 4
```

In this case the gue key is 65539 which is 0x00010003 so the account
key is 1 and the service key is 3.

### Allocation Algorithm

We use Kubernetes' opportunistic locking to allocate GUE keys. When a
key is needed, we increment the key value in the parent's status and
then Update() the parent. Using an Update() is important because the
update will fail if someone else got there first. In that case we read
the object again and retry the Update().

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
