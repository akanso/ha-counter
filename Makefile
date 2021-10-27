
# Image URL to use all building/pushing image targets
MY_REGISTRY=akanso
IMG ?= ${MY_REGISTRY}/counter:0.1

# Deploy pods/services in the configured Kubernetes cluster in ~/.kube/config
deploy:  check-registry
	kubectl create configmap etcd-config --from-literal=endpoints="http://etcd0:2379,http://etcd1:2379,http://etcd2:2379"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/counter-deploy.yaml
	kubectl create -f config/

clean: 
	kubectl delete -f config/
	kubectl delete configmap etcd-config

# Build the docker image
docker-build: check-registry
	docker build . -t ${IMG}
	@echo "updating  image patch file for counter resource"
	sed -i''  -e 's@image: .*@image: '"${IMG}"'@' ./config/counter-deploy.yaml

# Push the docker image
docker-push: check-registry
	docker push ${IMG}

check-registry:
ifndef MY_REGISTRY
	$(error MY_REGISTRY is not set, please set your registry, e.g.: make MY_REGISTRY=docker_ID docker-push)
endif