# Barrelman
Watches a remote clusters nodes for changes and updates local clusters service endpoints accordingly.

 
 
 

## Terms
* remote(-cluster) is always the cluster who's nodes are being watched
* local(-cluster) is always the cluster to manage services/endpoints in

## What it does
This only handles service objects labeled with `"tfw.io/barrelman": "true"`.

Watch for changes on service objects in _local-cluster_:
* Add/Modify: Add or update a matching endpoint object
    * Internal IPs from up to date list of nodes in _remote-cluster_
    * Port from `targetPort` of service
* Delete: Do nothing (kubernetes will clean up the endpoint automatically)

Watch for changes of nodes in _remote-cluster_:
* Add: Queue all service objects in _local-cluster_
* Modify: Queue all service objects in _local-cluster_
* Delete: Queue all service objects in _local-cluster_

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
FIXME
Probably needs RBAC role to read service objetcs and create service endpoint

## Remote cluster
Needs service account with "Kubernetes Engine Viewer" permission (to read node details)


