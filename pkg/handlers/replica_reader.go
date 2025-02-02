// Copyright (c) Alex Ellis 2017. All rights reserved.
// Copyright 2020 OpenFaaS Author(s)
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/openfaas/faas-netes/pkg/k8s"
	"github.com/openfaas/faas-provider/proxy"
	types "github.com/openfaas/faas-provider/types"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/client-go/listers/apps/v1"
	glog "k8s.io/klog"
)

// MakeReplicaReader reads the amount of replicas for a deployment
func MakeReplicaReader(config types.FaaSConfig, resolver proxy.BaseURLResolver, defaultNamespace string, lister v1.DeploymentLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)

		functionName := vars["name"]
		q := r.URL.Query()
		namespace := q.Get("namespace")

		lookupNamespace := defaultNamespace

		if len(namespace) > 0 {
			lookupNamespace = namespace
		}

		s := time.Now()

		function, err := getService(lookupNamespace, functionName, lister)
		if err != nil {
			log.Printf("Unable to fetch service: %s %s\n", functionName, namespace)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if function == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		//get true function replica
		replicaFunc, err := updateReplica(function.Name, config, resolver, r)
		if err != nil {
			log.Println("read replica failed: ", err)
		} else {
			//log.Printf("update %s function replicas %d to %d", function.Name, function.Replicas, replicaFunc.Replicas)
			function.Replicas = replicaFunc.Replicas
			function.AvailableReplicas = replicaFunc.AvailableReplicas
			function.InvocationCount = replicaFunc.InvocationCount
		}
		d := time.Since(s)
		log.Printf("Replicas: %s.%s, (%d/%d) %dms\n", functionName, lookupNamespace, function.AvailableReplicas, function.Replicas, d.Milliseconds())
		functionBytes, err := json.Marshal(function)
		if err != nil {
			glog.Errorf("Failed to marshal function: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to marshal function"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(functionBytes)
	}
}

// getService returns a function/service or nil if not found
func getService(functionNamespace string, functionName string, lister v1.DeploymentLister) (*types.FunctionStatus, error) {

	item, err := lister.Deployments(functionNamespace).
		Get(functionName)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	if item != nil {
		function := k8s.AsFunctionStatus(*item)
		if function != nil {
			return function, nil
		}
	}

	return nil, fmt.Errorf("function: %s not found", functionName)
}
