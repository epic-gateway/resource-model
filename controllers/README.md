There are two sets of controllers here. The first set belongs to the
controller-manager and the second set belongs to the node agent. The
controller manager controllers do things that need to be done once per
cluster, such as notifying EPIC of new services and endpoints. The
agent controllers do per-node setup, such as configuring the True
Ingress components.
