# See https://youtu.be/Oevqe9kKkKc for an example.

.PHONY: help bootstrap-minikube purge deploy k8s-data-namespace helm-es-service helm-cdb-service dockerize-and-push

SHELL=/bin/bash -o pipefail

# Update these with their correct versions.
ES_VERSION ?= 8.7.1
# remove the 'v'?
CDB_VERSION ?= v22.2.9
MINIKUBE_RAM ?= 5g
MINIKUBE_CPUS ?= 3
MINIKUBE_K8S_VERSION ?= 1.26.5 # https://kubernetes.io/docs/tasks/tools/install-kubectl-windows/#before-you-begin
MINIKUBE_DRIVER ?= virtualbox

help:
	@echo "Usage: make COMMAND"
	@echo ""
	@echo "Supported commands:"
	@echo "- bootstrap-minikube : bootstrap minikube cluster and install required addons"
	@echo "- deploy             : deploy a micro-service based full links 'R' us installation backed by CockroachDB and elastic search"
	@echo "- purge              : delete links 'R' us deployment including backing databases"
	@echo "- dockerize-and-push : dockerize and push all required images"
	@echo ""

# Make the cluster gets provisioned correctly, and that all addons are enabled. You may need to rerun some commands.
bootstrap-minikube:
	@echo "[minikube] bootstrapping (driver: ${MINIKUBE_DRIVER}, network-plugin: cni) cluster with kubernetes ${MINIKUBE_K8S_VERSION} and reserving ${MINIKUBE_RAM} of RAM and ${MINIKUBE_CPUS} CPU(s)"
	@minikube start --vm-driver=${MINIKUBE_DRIVER} --network-plugin=cni --kubernetes-version=${MINIKUBE_K8S_VERSION} --memory=${MINIKUBE_RAM} --cpus=${MINIKUBE_CPUS} --no-vtx-check 2>&1 | sed -e 's/^/ | /g'
#	@echo "[helm] bootstrapping" (no longer needed as of helm v3: https://helm.sh/docs/topics/v2_v3_migration/)
#	@helm init 2>&1 | sed -e 's/^/ | /g'
	@echo "[minikube] enabling addons: registry, ingress"
	@(minikube addons enable registry 2>&1 || true) | sed -e 's/^/ | /g'
	@(minikube addons enable ingress 2>&1 || true) | sed -e 's/^/ | /g'
	@echo "[minikube] enabling addons for Elasticsearch" # See https://github.com/elastic/helm-charts/tree/main/elasticsearch/examples/minikube#requirements
	@(minikube addons enable default-storageclass 2>&1 || true) | sed -e 's/^/ | /g'
	@(minikube addons enable storage-provisioner 2>&1 || true) | sed -e 's/^/ | /g'
	@echo "[calico] installing manifests" # see https://docs.tigera.io/calico/latest/getting-started/kubernetes/self-managed-onprem/onpremises
#	@curl https://raw.githubusercontent.com/projectcalico/calico/v3.26.0/manifests/calico.yaml -O
	@kubectl apply -f calico.yaml 2>&1 | sed -e 's/^/ | /g'
	@echo
	@echo "IMPORTANT NOTICE:"
	@echo "Please make sure to add '"`minikube ip`":5000' as a trusted"
	@echo "insecure registry to your docker's configuration options and"
	@echo "restart the docker daemon!"
	@echo
	@echo 'On Linux, you can do this by editing /etc/docker/daemon.json'
	@echo 'and adding the following section:'
	@echo '  {'
	@echo '    "insecure-registries" : ['
	@echo '      "'`minikube ip`':5000"'
	@echo '    ]'
	@echo '  }'
	@echo
	@echo 'On OSX and Windows you can right-click the docker icon, go to'
	@echo '"preferences" and then click on the "Daemon" tab'

purge:
	@echo "[kubectl] removing helm deployments for CDB/ES"
	@helm uninstall elastic 2>&1 | sed -e 's/^/ | /g' || true
	@helm uninstall cockroachdb 2>&1 | sed -e 's/^/ | /g' || true
	@echo "[kubectl] removing remaining resources"
	@kubectl delete -f . 2>&1 | sed -e 's/^/ | /g' || true
# docker system prune
# wait a bit since the above command will write to some logs
# minikube delete

deploy: k8s-data-namespace helm-es-service helm-cdb-service
	@echo "[kubectl] deploying linksrus"
	@kubectl apply -f . 2>&1 | sed -e 's/^/ | /g'

k8s-data-namespace:
	@if [[ -z `kubectl get ns | grep data` ]]; then \
		echo "[kubectl] applying namespace manifests";\
		kubectl apply -f 01-namespaces.yaml 2>&1 | sed -e 's/^/ | /g';\
	fi

# See https://github.com/elastic/helm-charts/tree/main/elasticsearch
helm-es-service: k8s-data-namespace
	@if [[ `kubectl -n linksrus-data get pods -l release=elasticsearch 2>/dev/null | wc -l` -eq '0' ]]; then \
		echo "[helm] installing elasticsearch (${ES_VERSION})";\
		echo "[helm] adding chart repos";\
		helm repo add elastic https://helm.elastic.co 2>&1 | sed -e 's/^/ | /g';\
		helm repo update 2>&1 | sed -e 's/^/ | /g'; \
		helm install elasticsearch elastic/elasticsearch \
			--namespace=linksrus-data \
			--values chart-settings/es-settings.yaml \
			--set imageTag=${ES_VERSION} 2>&1 | sed -e 's/^/ | /g';\
	fi

# See https://www.cockroachlabs.com/docs/stable/deploy-cockroachdb-with-kubernetes-insecure.html
helm-cdb-service: k8s-data-namespace
	@if [[ `kubectl -n linksrus-data get pods -l release=cdb 2>/dev/null | wc -l` -eq '0' ]]; then \
		echo "[helm] installing cockroackdb (${CDB_VERSION})";\
		helm repo add cockroachdb https://charts.cockroachdb.com/ 2>&1 | sed -e 's/^/ | /g'; \
		helm repo update 2>&1 | sed -e 's/^/ | /g'; \
		helm install cdb cockroachdb/cockroachdb \
			--namespace=linksrus-data \
			--values chart-settings/cdb-settings.yaml \
			--set ImageTag=${CDB_VERSION} 2>&1 | sed -e 's/^/ | /g';\
	fi

# kubectl -n linksrus-data get pods
# kubectl -n linksrus-data logs cdb-ensure-schema-<id>
# kubectl -n linksrus-data delete pod cdb-ensure-schema-<id>

dockerize-and-push:
	@make -C ../../cdb dockerize-and-push
	@make -C ../../ dockerize-and-push
