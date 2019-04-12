# Admission Control Example

This project presented how to utilize [Admission Control](https://kubernetes.io/blog/2019/03/21/a-guide-to-kubernetes-admission-controllers/) to implement a graceful shutdown of [Etcd](https://github.com/etcd-io/etcd).

# Why not [prestop](https://kubernetes.io/docs/tasks/configure-pod-container/attach-handler-lifecycle-event/#define-poststart-and-prestop-handlers)

- Exiting time limitation. 
- Can't rollback exit command.

# Example

The following case uses my image by default, you can build yours by `docker build . -t YOUR_IMAGE_NAME`. Then change the reference of [deployment.yaml](deployment.yaml)

```bash
$ git clone git@github.com:dreampuf/etcd-admission-control-example.git
$ ./bootstrap.sh
Generating a 2048 bit RSA private key
..............................................................................................+++
............................................................................................+++
writing new private key to 'ca.key'
-----
Generating RSA private key, 2048 bit long modulus
......................................+++
......+++
e is 65537 (0x10001)
Signature ok
subject=/CN=etcd-admission-control.default.svc
Getting CA Private Key
secret/etcd-admission-key created
deployment.extensions/etcd-admission-control created
service/etcd-admission-control created
validatingwebhookconfiguration.admissionregistration.k8s.io/etcd-webhook created
Etcd Admission Control has been deployed.
$ kubectl get all
NAME                                         READY   STATUS    RESTARTS   AGE
pod/etcd-admission-control-ff9494767-cjgq5   1/1     Running   0          8s

NAME                             TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE
service/etcd-admission-control   ClusterIP   10.99.230.58   <none>        443/TCP   8s
service/kubernetes               ClusterIP   10.96.0.1      <none>        443/TCP   3d16h

NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/etcd-admission-control   1/1     1            1           8s

NAME                                               DESIRED   CURRENT   READY   AGE
replicaset.apps/etcd-admission-control-ff9494767   1         1         1       8s
$ kubectl create -f etcd-basic.yaml
service/etcd created
service/etcd-cluster created
statefulset.apps/etcd created

$ kubectl exec -it etcd-2 -- etcdctl put a "$(date)"
OK

$ kubectl delete pod/etcd-2
pod "etcd-2" deleted
$ kubectl delete pod/etcd-1
pod "etcd-1" deleted
$ kubectl exec -it etcd-0 -- etcdctl get a
a
Fri Apr 12 03:49:43 EDT 2019

## Open another tab
$ kubectl logs -f deployment.apps/etcd-admission-control
2019/04/12 07:47:54 http server started: 0.0.0.0:8443
2019/04/12 07:49:51 etcd-2
2019/04/12 07:49:52 sent memberremove request success: etcd-2
2019/04/12 07:49:56 etcd-2
2019/04/12 07:49:56 no validated member
2019/04/12 07:50:31 etcd-1
2019/04/12 07:50:31 sent memberremove request success: etcd-1
2019/04/12 07:50:36 etcd-1
2019/04/12 07:50:36 no validated member
```

# Credit

This project inspired by [admission-controller-webhook-demo](https://github.com/stackrox/admission-controller-webhook-demo) and [etcd-operator](https://github.com/coreos/etcd-operator). 
