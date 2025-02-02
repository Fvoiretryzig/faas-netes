// Copyright (c) Alex Ellis 2017. All rights reserved.
// Copyright 2020 OpenFaaS Author(s)
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/openfaas/faas-provider/proxy"
	types "github.com/openfaas/faas-provider/types"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	v1 "k8s.io/client-go/listers/apps/v1"
	glog "k8s.io/klog"

	"github.com/openfaas/faas-netes/pkg/k8s"
)

// MakeFunctionReader handler for reading functions deployed in the cluster as deployments.
func MakeFunctionReader(config types.FaaSConfig, resolver proxy.BaseURLResolver, defaultNamespace string, deploymentLister v1.DeploymentLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		namespace := q.Get("namespace")

		lookupNamespace := defaultNamespace

		if len(namespace) > 0 {
			lookupNamespace = namespace
		}

		if lookupNamespace == "kube-system" {
			http.Error(w, "unable to list within the kube-system namespace", http.StatusUnauthorized)
			return
		}

		functions, err := getServiceList(config, resolver, r, lookupNamespace, deploymentLister)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		functionBytes, err := json.Marshal(functions)
		if err != nil {
			glog.Errorf("Failed to marshal functions: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to marshal functions"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(functionBytes)
	}
}

func getServiceList(config types.FaaSConfig, resolver proxy.BaseURLResolver, r *http.Request, functionNamespace string, deploymentLister v1.DeploymentLister) ([]types.FunctionStatus, error) {
	functions := []types.FunctionStatus{}

	sel := labels.NewSelector()
	req, err := labels.NewRequirement("faas_function", selection.Exists, []string{})
	if err != nil {
		return functions, err
	}
	onlyFunctions := sel.Add(*req)

	res, err := deploymentLister.Deployments(functionNamespace).List(onlyFunctions)

	if err != nil {
		return nil, err
	}

	for _, item := range res {
		if item != nil {
			function := k8s.AsFunctionStatus(*item)
			if function != nil {
				//log.Println("[getServiceList] ", function.Name, "Labels: ", function.Labels)
				//update replica value(get from watchdog)
				replicaFunc, err := updateReplica(function.Name, config, resolver, r)
				if err != nil {
					log.Println("read replica failed: ", err)
				} else {
					//log.Printf("update %s function replicas %d to %d", function.Name, function.Replicas, replicaFunc.Replicas)
					function.Replicas = replicaFunc.Replicas
					function.AvailableReplicas = replicaFunc.AvailableReplicas
					function.InvocationCount = replicaFunc.InvocationCount
				}
				functions = append(functions, *function)
			}
		}
	}

	return functions, nil
}
