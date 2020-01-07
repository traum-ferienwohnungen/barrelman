# Barrelman
Watches a remote clusters nodes and services for changes and updates local clusters services and endpoints
accordingly.

Developed and used to keep service reachable via in-cluser URLs from multiple clusters. This is "hacked" by creating
dummy services in cluster A pointing to node IPs and ports of cluster B.

Currently the authentication towards the remote cluster is tightened to Google Kubernetes Engine clusters.

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

Watch for changes of service objects in _local-cluster_:
* Add/Modify: do nothing
* Delete: Check if there is a corresponding service in _remote-cluster_ and add a dummy as needed

Watch for changes of services objects in _remote-cluster_:
* Add: Create a dummy service in _local-cluster_ (to be picked up by [NodeEndpointController](#NodeEndpointController))
    * Create namespace in _local-cluster_ (if needed)
    * Create service in _local-cluster_ if it does not exist, update as in modify if it does
* Modify: Update corresponding service object in _local-cluster_
    * All service ports of the remote service
* Delete: Remove dummy service in _local-cluster_ if it was created by barrelman

### What to expect
Imaging there is cluster X and Y (Nodes Xn and Yn) with barrelman running as Xb and Yb.


#### Manual service creation
* Create Service "foo/baz" (barrelman label, targetPort == NodePort of some service in X) in Y
    * Endpoint(s) "foo/baz" are created in Y (pointing to Xn)
* Change targetPort of "foo/baz" in Y
    * Endpoint(s) "foo/baz" are in Y are updated accordingly

#### Auto service creation
* Create Service "foo/bar" (type NodePort) in X
    * Namespace "foo" is created in Y
    * Service "foo/bar" (Type: ClusterIP, targetPort == NodePort of "foo/bar" in X) is created in Y
    * Endpoint(s) "foo/bar" are created in Y (pointing to Xn:nodePort)
* Change NodePort of service "foo/bar" in X
    * targetPort of service "foo/bar" in Y is updated accordingly
    * Endpoint(s) "foo/bar" in Y are updated accordingly
* Delete Service "foo/bar" in X
    * Service "foo/bar" in Y is deleted
    * Endpoint(s) "foo/bar" in Y are deleted



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
See [rbac.yaml](helm/barrelman/templates/rbac.yaml)

## Remote cluster
Needs service account with "Kubernetes Engine Viewer" IAM permission (to read node and service details).

To create a service account, use:
```bash
PROJECT="gcp-project"
gcloud --project="$PROJECT" iam service-accounts create barrelman --display-name barrelman

# Grant Kubernetes Engine Viewer permission
gcloud projects add-iam-policy-binding $PROJECT \
    --member serviceAccount:barrelman@${PROJECT}.iam.gserviceaccount.com --role "roles/container.viewer"

# Create a service account key (to be used in CI/CD)
gcloud iam service-accounts keys create service-account.json \
    --iam-account=barrelman@${PROJECT}.iam.gserviceaccount.com

# Base64 encode the service account, store the output in GitLab CI variable REMOTE_SERVICE_ACCOUNT
base64 -w0 < service-account.json
```


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
