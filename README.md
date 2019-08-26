# Barrelman
Watches a remote clusters nodes and services for changes and updates local clusters services and endpoints
accordingly.

 
 
 

## Terms
* remote(-cluster) is always the cluster who's nodes and services are being watched
* local(-cluster) is always the cluster to manage services and endpoints in

## What it does
Barrelman consists of two different controller routines, watching for different events in _remote-cluster_.

### NodeEndpointController
This only handles service with the label `tfw.io/barrelman` set to `"true"` or `"managed-resource"`
(see [ServiceController](#ServiceController) for the latter).

Watch for changes of service objects in _local-cluster_:
* Add/Modify: Add or update a matching endpoint object
    * Internal IPs from up to date list of nodes in _remote-cluster_
    * Port from `targetPort` of service
* Delete: Do nothing (kubernetes will clean up the endpoint automatically)

Watch for changes of nodes in _remote-cluster_:
* Add: Queue all service objects in _local-cluster_ for endpoint updates
* Modify: Queue all service objects in _local-cluster_ for endpoint updates
* Delete: Queue all service objects in _local-cluster_ for endpoint updates

### ServiceController
ServiceController operates on services in _remote-cluster_ if they are not within a ignored namespace
(`--ignore-namespace`, `kube-system` is ignored by default) and not ignored via annotation
(`tfw.io/barrelman: ignore`).

Services in _local-cluster_ are only updated/deleted if they are labeled with
(`tfw.io/barrelman: managed-resource`). Namespaces created by barrelman are never removed.

Watch for changes of services objects in _remote-cluster_:
* Add: Create a dummy service in _local-cluster_ (to be picked up by [NodeEndpointController](#NodeEndpointController))
    * Create namespace in _local-cluster_ (if needed)
    * Create service in _local-cluster_ if it does not exist, update as in modify if it does
* Modify: Update corresponding service object in _local-cluster_
    * All service ports of the remote service
* Delete: Remove dummy service in _local-cluster_ if it was created by barrelman

**Constraints:**

Services in _local-cluster_ are not watched by ServiceController. That means that if a _local-cluster_ service
which prevented barrelman from creating a _managed-resource_ gets removed, the _managed-resource_ will not be
created until the corresponding service object in _remote-cluster_ changes.

I'm not sure if this is smart but it's the only case in which a _local-cluster_ watcher would be needed. So it
takes a lot of complexity out of barrelman to leave this constraint.

If in doubt, wo could remove the check for equal `ResourceVersion` in ServiceConroller event handler and lower the
resync period (that would, of cause, mean more traffic towards kubernetes API).


# Run
Local cluster may be specified via `local-kubeconfig` and `local-context`. If omitted, in-cluster credentials will
be used (where possible).

Remote cluster must be defined via `remote-project`, `remote-zone` and `remote-cluster-name`. Cluster credentials and
config (API Host etc.) will then be auto generated via a Google APIs using the service account provided via the 
environment Variable `GOOGLE_APPLICATION_CREDENTIALS`.

```bash
barrelman -v 3 \
  -local-kubeconfig ~/.kube/config \
  -local-context "gke_gcp-project_region-and-zone_local-cluster-name" \
  -remote-cluster-name remote-cluster-name \
  -resync-period 1m
```

# Permissions:
## Local cluster
See [rbac.yaml](k8s/barrelman/templates/rbac.yaml)

## Remote cluster
Needs service account with "Kubernetes Engine Viewer" IAM permission (to read node and service details)


# Development
## .git/hooks/pre-commit
```bash
#!/bin/bash
STAGED_GO_FILES=$(git diff --cached --name-only | grep ".go$")

if [[ "$STAGED_GO_FILES" = "" ]]; then
  exit 0
fi

exec golangci-lint run --fix
```
