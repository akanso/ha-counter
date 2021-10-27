kubectl delete configmap etcd-config 
kubectl delete -f ./etcd.yaml
kubectl delete -f ./counter-svc.yaml
kubectl delete -f ./counter-deploy.yaml
sleep 10
docker rmi akanso/counter:latest