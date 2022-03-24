// Copyright (c) Alex Ellis 2017. All rights reserved.
// Copyright 2020 OpenFaaS Author(s)
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package handlers

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/openfaas/faas-provider/httputil"
	"github.com/openfaas/faas-provider/proxy"

	"github.com/gorilla/mux"
	"github.com/openfaas/faas-provider/types"
)

// MakeReplicaUpdater updates desired count of replicas
func MakeReplicaUpdater(config types.FaaSConfig, resolver proxy.BaseURLResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Update replicas")

		vars := mux.Vars(r)

		functionName := vars["name"]
		/*q := r.URL.Query()
		namespace := q.Get("namespace")

		lookupNamespace := defaultNamespace

		if len(namespace) > 0 {
			lookupNamespace = namespace
		}*/

		req := types.ScaleServiceRequest{}

		if r.Body != nil {
			defer r.Body.Close()
			bytesIn, _ := ioutil.ReadAll(r.Body)
			marshalErr := json.Unmarshal(bytesIn, &req)
			if marshalErr != nil {
				w.WriteHeader(http.StatusBadRequest)
				msg := "Cannot parse request. Please pass valid JSON."
				w.Write([]byte(msg))
				log.Println(msg, marshalErr)
				return
			}
		}
		//send request to watch dog
		proxyClient := NewProxyClientFromConfig(config)
		functionAddr, resolveErr := resolver.Resolve(functionName)
		if resolveErr != nil {
			// TODO: Should record the 404/not found error in Prometheus.
			log.Printf("resolver error: no endpoints for %s: %s\n", functionName, resolveErr.Error())
			httputil.Errorf(w, http.StatusServiceUnavailable, "No endpoints available for: %s.", functionName)
			return
		}

		proxyReq, err := buildProxyRequest(r, functionAddr) //no params for replicaUpdater handler
		if err != nil {
			httputil.Errorf(w, http.StatusInternalServerError, "Failed to resolve service: %s.", functionName)
			return
		}
		if proxyReq.Body != nil {
			defer proxyReq.Body.Close()
		}

		ctx := r.Context()
		response, err := proxyClient.Do(proxyReq.WithContext(ctx)) //send request to watchdog
		if err != nil {
			log.Printf("error with proxy request to: %s, %s\n", proxyReq.URL.String(), err.Error())

			httputil.Errorf(w, http.StatusInternalServerError, "Can't reach service for: %s.", functionName)
			return
		}
		if response.Body != nil {
			defer response.Body.Close()
		}
		log.Printf("Set replicas - %s, %d\n", functionName, req.Replicas)

		/*options := metav1.GetOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
		}

		deployment, err := clientset.AppsV1().Deployments(lookupNamespace).Get(context.TODO(), functionName, options)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Unable to lookup function deployment " + functionName))
			log.Println(err)
			return
		}

		oldReplicas := *deployment.Spec.Replicas
		replicas := int32(req.Replicas)

		log.Printf("Set replicas - %s %s, %d/%d\n", functionName, lookupNamespace, replicas, oldReplicas)

		deployment.Spec.Replicas = &replicas

		_, err = clientset.AppsV1().Deployments(lookupNamespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Unable to update function deployment " + functionName))
			log.Println(err)
			return
		}
		w.WriteHeader(http.StatusAccepted)*/
		w.WriteHeader(response.StatusCode)
	}
}
