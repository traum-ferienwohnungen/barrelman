package controller

import (
	"barrelman/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

// getLocalAction returns the type of action (ActionType) to take on local service
func getLocalAction(remoteExists bool, remoteSvc *v1.Service, localExists bool, localSvc *v1.Service) ActionType {
	if remoteExists && utils.ResponsibleForService(remoteSvc) {
		klog.Infof("%s/%s responsible for remote", remoteSvc.GetNamespace(), remoteSvc.GetName())

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.Warningf("%s/%s we don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.Warningf("%s/%s not responsible for local service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			klog.Infof("%s/%s remote and local exist, UPDATE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeUpdate
		} else {
			klog.Infof("%s/%s local does not exist, ADD", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeAdd
		}
	}

	if !remoteExists || !utils.ResponsibleForService(remoteSvc) {
		klog.Infof("%s/%s not responsible for remote", remoteSvc.GetNamespace(), remoteSvc.GetName())

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.Warningf("%s/%s we don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.Infof("%s/%s local exists but not responsible, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}
			klog.Infof("%s/%s local does exist, DELETE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeDelete
		}
	}

	return ActionTypeNone
}
