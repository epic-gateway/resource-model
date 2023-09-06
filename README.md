The EPIC operator manages Envoy pods based on our custom resources. The
data model and operator code are scaffolded by
https://operatorframework.io/.

## Useful Commands

To generate a new custom resource definition:
```
$ operator-sdk create api --group epic --version v1 --kind EPIC
```

To generate a webhook:
```
$ operator-sdk create webhook --group epic --version v1 --kind EPIC --defaulting --programmatic-validation
```

## ID Allocation

Numeric ID's (like Tunnel ID) are allocated
from fields in their parent CR Status. Tunnel IDs are unique across
the whole system so they're allocated from the epic singleton
object. You can examine the current value by dumping the contents of
the object:

```
$ kubectl get -n epic epics.epic.acnodal.io epic -o yaml
apiVersion: epic.acnodal.io/v1
kind: EPIC
metadata:
  ... snip ...
  name: epic
  namespace: epic
  resourceVersion: "22829"
  uid: e6a50ecf-1829-4e47-8537-abfd9b6a9146
spec: {}
status:
  current-tunnel-id: 4
```

The ```current-tunnel-id``` field is the most recent value that we
assigned to a tunnel ID (within a Service object).

```
$ kubectl get -n epic-root loadbalancers.epic.acnodal.io sample-acnodal -o yaml
apiVersion: epic.acnodal.io/v1
kind: LoadBalancer
metadata:
  ... snip ...
  name: sample-acnodal
  namespace: epic-root
  resourceVersion: "2584"
  uid: 1a8522b9-70a4-4a9c-bdd7-ba912a61170a
spec:
  service-id: 1
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
      epic-address: 192.168.1.40
      epic-port:
        appProtocol: gue
        port: 6080
        protocol: UDP
      tunnel-id: 2
    192.168.1.25:
      epic-address: 192.168.1.40
      epic-port:
        appProtocol: gue
        port: 6080
        protocol: UDP
      tunnel-id: 1
  proxy-ifindex: 16
  proxy-ifname: veth67060f1f
```

Each tunnel in the status has a tunnel ID that was allocated from the
EPIC configuration singleton.

### Allocation Algorithm

We use Kubernetes' opportunistic locking to allocate group/service
IDs. When an ID is needed, we increment the current ID value in the
parent's status and then Update() the parent. Using Update() (and not
Patch()) is important because Update() will fail if another request
got there first. In that case we read the object again and retry the
Update().

## Namespaces
The core namespace is "epic" - our processes run there, and system-scoped objects like the EPIC config singleton and LBServiceGroup CRs also live there.
Users can create their own "user namespaces" using epicctl. Each user namespace is entirely independent of the others. User namespaces are prefixed with epicv1.ProductName + "-". See epicv1.ProductName, epicv1.AccountNamespace().

## Naming
Because our data model is implemented using k8s custom resources, our objects' names have to live with the k8s rules for naming objects (whatever those are).

## Objects

### Account
Each user namespace has one Account CR to store configuration that is relevant to that user namespace.
The "acme-widgets" account's namespace, for example, would be "epic-acme-widgets".

### LBServiceGroup
Service Groups are stored in their owning accounts' namespaces.

### GWProxy
Proxies are stored in their owning accounts' namespaces.

### LoadBalancer
Load Balancers are stored in their owning accounts' namespaces.
