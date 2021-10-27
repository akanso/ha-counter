kubectl create configmap etcd-config --from-literal=endpoints="http://etcd0:2379,http://etcd1:2379,http://etcd2:2379"
kubectl create -f ./etcd.yaml
kubectl create -f ./counter-svc.yaml
kubectl create -f ./counter-deploy.yaml
#hey -n 100 -c 4 -q <ip>:<port>
# hey -n 100 -c 4 http://10.152.183.208:8080
# for i in {1..10}; do echo "iteration $i"; curl http://10.152.183.208:8080; sleep 1;done
#  k exec -ti etcd0 --  etcdctl get counter