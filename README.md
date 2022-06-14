The EPIC operator manages Envoy pods based on our custom resources. The
data model and operator code are scaffolded by
https://operatorframework.io/.

## Developer Setup

This project depends on modules that are not publicly visible, so you
need to configure git so that it can clone our private module repos
even when it is being run by commands like "go get". There are two
ways that this can happen: inside of docker (for example, when you're
building images to run in k8s), and outside of docker (for example,
when you're running go programs at the command line for debugging
purposes).

To set up "inside docker" access, first create a GitLab Personal
Access Token that can read repos. You can do that at
https://gitlab.com/profile/personal_access_tokens . Then define an
environment variable in your account called GITLAB_PASSWORD that
contains the token, and another called GITLAB_USER that contains
"oauth2". This project's Makefile will pass these variables into
Docker which will use them to clone our private module repos when it
builds our go programs.

To set up "outside docker" access, configure git to use the token you
created in the previous paragraphs to clone Acnodal repos on
gitlab. This can be done using a ~/.netrc file containing:

 machine gitlab.com login oauth2 password {put your token here}

You can test this by running the command:

 $ go get gitlab.com/acnodal/epic/resource-model@v0.1.0-pre1

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
