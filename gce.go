package main

import (
	"context"
	"encoding/base64"
	"fmt"

	"k8s.io/client-go/tools/clientcmd/api"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

func NewGKEClientset(project, zone, clusterName string) (*kubernetes.Clientset, error) {
	ctx := context.Background()

	// See https://cloud.google.com/docs/authentication/.
	// Use GOOGLE_APPLICATION_CREDENTIALS environment variable to specify
	// a service account key file to authenticate to the API.
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("Could not get authenticated client: %v", err)
	}

	containerService, err := container.New(hc)
	if err != nil {
		return nil, fmt.Errorf("Could not initialize gke client: %v", err)
	}
	cluster, err := containerService.Projects.Zones.Clusters.Get(project, zone, clusterName).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("cluster %q could not be found in project %q, zone %q: %v", clusterName, project, zone, err)
	}

	return NewClientsetFromGKECluster(cluster)
}

func NewClientsetFromGKECluster(cluster *container.Cluster) (*kubernetes.Clientset, error) {
	decodedClusterCaCertificate, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, fmt.Errorf("decode cluster CA certificate error:", err)
	}

	config := &rest.Config{
		Host: "https://" + cluster.Endpoint,
		AuthProvider: &api.AuthProviderConfig{
			Name: "gcp",
		},
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
			CAData:   decodedClusterCaCertificate,
		},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s client set from config: %s\n", err)
	}

	return clientset, nil
}
