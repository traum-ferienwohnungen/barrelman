package controller

import (
	"barrelman/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

// getLocalAction returns the type of action (ActionType) to take on local service
func getLocalAction(remoteExists bool, remoteSvc *v1.Service, localExists bool, localSvc *v1.Service) ActionType {
	if remoteExists && utils.ResponsibleForService(remoteSvc) {
		klog.Infof("remote: %s/%s I'm responsible", remoteSvc.GetNamespace(), remoteSvc.GetName())

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.Warningf("local: %s/%s I don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.Warningf("local: %s/%s not responsible for service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			klog.Infof("remote,local: %s/%s both exist, UPDATE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeUpdate
		} else {
			klog.Infof("local: %s/%s does not exist, ADD", remoteSvc.GetNamespace(), remoteSvc.GetName())
			return ActionTypeAdd
		}
	}

	if !remoteExists || !utils.ResponsibleForService(remoteSvc) {
		// It's not completely sure that remoteSvc is not nil, so we can't log namespace and name
		klog.Infoln("remote: not responsible")

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.Warningf("local: %s/%s I don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.Infof("local: %s/%s exists but not responsible, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}
			klog.Infof("local: %s/%s does exist, DELETE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeDelete
		}
	}

	return ActionTypeNone
}
