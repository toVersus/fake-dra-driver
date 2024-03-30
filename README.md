# Fake Resource Allocation for Dynamic Resource Allocation (DRA) 

This repository contains a sample implementation of resource driver for use with the [Dynamic Resource Allocation (DRA)](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/) feature of Kubernetes based on [kubernetes-sigs/dra-example-driver](https://github.com/kubernetes-sigs/dra-example-driver).

It is intended to demonstrate best-practices for how to construct a DRA resource driver and wrap it in a [helm chart](https://helm.sh/). It can be used as a starting point for implementing a driver for your own set of resources.

## Quickstart and Demo

Before diving into the details of how this example driver is constructed, it's useful to run through a quick demo of it in action.

The driver itself provides access to a set of mock Fake devices, and this demo walks through the process of building and installing the driver followed by
running a set of workloads that consume these Fakes.

The procedure below has been tested and verified on both Linux and Mac.

### Prerequisites

* [GNU Make 3.81+](https://www.gnu.org/software/make/)
* [GNU Tar 1.34+](https://www.gnu.org/software/tar/)
* [docker v20.10+](https://docs.docker.com/engine/install/)
* [kind v0.17.0+](https://kind.sigs.k8s.io/docs/user/quick-start/)
* [helm v3.7.0+](https://helm.sh/docs/intro/install/)
* [kubectl v1.18+](https://kubernetes.io/docs/reference/kubectl/)

### Demo

We start by first cloning this repository and `cd` ing into its `demo` subdirectory. All of the scripts and example Pod specs used in this demo are contained here, so take a moment to browse through the various files and see what's available:

```sh
git clone https://github.com/toVersus/fake-dra-driver.git
cd fake-dra-driver/demo
```

From here we will build the image for the example resource driver:

```sh
./build-driver.sh
```

And create a `kind` cluster to run it in:

```sh
./create-cluster.sh
```

Once the cluster has been created successfully, double check everything is coming up as expected:

```console
❯ kubectl get pods -A
NAMESPACE            NAME                                                            READY   STATUS    RESTARTS   AGE
kube-system          coredns-5d78c9869d-zcm54                                        1/1     Running   0          49m
kube-system          coredns-5d78c9869d-zpcvv                                        1/1     Running   0          49m
kube-system          etcd-fake-dra-driver-cluster-control-plane                      1/1     Running   0          49m
kube-system          kindnet-5rd6v                                                   1/1     Running   0          49m
kube-system          kindnet-rqclr                                                   1/1     Running   0          49m
kube-system          kube-apiserver-fake-dra-driver-cluster-control-plane            1/1     Running   0          49m
kube-system          kube-controller-manager-fake-dra-driver-cluster-control-plane   1/1     Running   0          49m
kube-system          kube-proxy-tb7rx                                                1/1     Running   0          49m
kube-system          kube-proxy-tz5rf                                                1/1     Running   0          49m
kube-system          kube-scheduler-fake-dra-driver-cluster-control-plane            1/1     Running   0          49m
local-path-storage   local-path-provisioner-5d7949c7d4-jf6z9                         1/1     Running   0          49m
```

And then install the example resource driver via `helm`:

```sh
helm upgrade -i \
  --create-namespace \
  --namespace fake-system \
  fake-dra-driver \
  deployments/helm/fake-dra-driver
```

Double check the driver components have come up successfully:

```console
❯ kubectl get pod -n fake-system
NAME                                          READY   STATUS    RESTARTS   AGE
fake-dra-driver-controller-7b46b9775d-g66cg   1/1     Running   0          50m
fake-dra-driver-kubeletplugin-7hk7g           1/1     Running   0          50m
```

And show the initial state of available GPU devices on the worker node:

```console
$ kubectl describe -n fake-system nas/fake-dra-driver-cluster-worker
(...)
Spec:
  Allocatable Device:
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-a1e41d9f-5b3a-d8f8-1b08-177352484a68
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-5d25fe96-37be-6082-ae92-d398d7d7038d
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-9fe9fc83-a0ec-2e8a-8749-e1a9423947d4
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-68cb1988-7257-2eff-4c68-f58c5ba83bd8
(...)
```

Next, deploy four example apps that demonstrate how `ResourceClaim`s, `ResourceClaimTemplate`s, and custom `ClaimParameter` objects can be used to request access to resources in various ways:

```sh
kubectl apply --filename=fake-test{1,2,3,4}.yaml
```

And verify that they are coming up successfully:

```console
$ kubectl get pod -A
NAMESPACE   NAME   READY   STATUS              RESTARTS   AGE
(...)
test1                pod0                                                            1/1     Running   0          83s
test1                pod1                                                            1/1     Running   0          83s
test2                pod0                                                            2/2     Running   0          83s
test3                pod0                                                            1/1     Running   0          83s
test3                pod1                                                            1/1     Running   0          83s
test4                pod0                                                            1/1     Running   0          83s
(...)
```

Use your favorite editor to look through each of the `fake-test{1,2,3,4}.yaml` files and see what they are doing.

Then dump the logs of each app to verify that Fakes were allocated to them according to these semantics:

```sh
for example in $(seq 1 4); do \
  echo "test${example}:"
  for pod in $(kubectl get pod -n test${example} --output=jsonpath='{.items[*].metadata.name}'); do \
    for ctr in $(kubectl get pod -n test${example} ${pod} -o jsonpath='{.spec.containers[*].name}'); do \
      echo "${pod} ${ctr}:"
      kubectl logs -n test${example} ${pod} -c ${ctr}| grep FAKE_DEVICE
    done
  done
  echo ""
done
```

This should produce output similar to the following:

```console
test1:
pod0 ctr0:
export FAKE_DEVICE_0='FAKE-9fe9fc83-a0ec-2e8a-8749-e1a9423947d4'
pod1 ctr0:
export FAKE_DEVICE_0='FAKE-68cb1988-7257-2eff-4c68-f58c5ba83bd8'

test2:
pod0 ctr0:
export FAKE_DEVICE_0='FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362'
pod0 ctr1:
export FAKE_DEVICE_0='FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362'

test3:
pod0 ctr0:
export FAKE_DEVICE_0='FAKE-5d25fe96-37be-6082-ae92-d398d7d7038d'
pod1 ctr0:
export FAKE_DEVICE_0='FAKE-5d25fe96-37be-6082-ae92-d398d7d7038d'

test4:
pod0 ctr0:
export FAKE_DEVICE_0='FAKE-a1e41d9f-5b3a-d8f8-1b08-177352484a68'
export FAKE_DEVICE_1='FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5'
export FAKE_DEVICE_2='FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc'
export FAKE_DEVICE_3='FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a'
```

In this example resource driver, no "actual" Fakes are made available to any containers. Instead, a set of environment variables are set in each container to indicate which Fakes *would* have been injected into them by a real resource driver.

You can use the UUIDs of the Fakes set in these environment variables to verify that they were handed out in a way consistent with the semantics shown in the figure above.

Likewise, looking at the `Prepared Devices` section of the `NodeAllocationState` object on the worker node will show which Fakes have been allocated to a given `ResourceClaim` by the resource driver:

```console
$ kubectl describe -n fake-system nas/fake-dra-driver-cluster-worker
(...)
Spec:
  (...)
  Prepared Devices:
    08a6552a-e762-4a28-89da-36a9df8baeab:
      Fake:
        Devices:
          Uuid:  FAKE-a1e41d9f-5b3a-d8f8-1b08-177352484a68
          Uuid:  FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
          Uuid:  FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
          Uuid:  FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
    8fdafcb6-c9bf-4a9c-ac7f-01ce1a7417fe:
      Fake:
        Devices:
          Uuid:  FAKE-5d25fe96-37be-6082-ae92-d398d7d7038d
    a10aa390-f1f2-4ebd-aca2-afa11a9ae12f:
      Fake:
        Devices:
          Uuid:  FAKE-68cb1988-7257-2eff-4c68-f58c5ba83bd8
    a681b6e2-024a-42c2-8a0f-97346895aff3:
      Fake:
        Devices:
          Uuid:  FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
    c2f40247-5005-4004-982e-f86ebe37eb3d:
      Fake:
        Devices:
          Uuid:  FAKE-9fe9fc83-a0ec-2e8a-8749-e1a9423947d4
```

Once you have verified everything is running correctly, delete all of the example apps:

```sh
kubectl delete --wait=false --filename=fake-test{1,2,3,4}.yaml
```

Wait for them to terminate and show that the `Prepared Devices` section of the `NodeAllocationState` object on the worker node is now back to its initial state:

```console
$ kubectl describe -n fake-system nas/fake-dra-driver-cluster-worker
(...)
Spec:
  Allocatable Device:
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-a1e41d9f-5b3a-d8f8-1b08-177352484a68
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-5d25fe96-37be-6082-ae92-d398d7d7038d
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-9fe9fc83-a0ec-2e8a-8749-e1a9423947d4
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-68cb1988-7257-2eff-4c68-f58c5ba83bd8
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
    Fake:
      Name:  LATEST-FAKE-MODEL
      Uuid:  FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
(...)
```

Next, deploy another example app that demonstrate how dynamic resource allocation like MIG (Multi Instance GPU) can be made:

```sh
kubectl apply --filename=fake-test5.yaml
```

Likewise, looking at the `Allocated Claims` section of the `NodeAllocationState` object on the worker node will show which Fakes will be allocated to a given `ResourceClaim` by the resource driver:

```console
$ kubectl describe -n fake-system nas/fake-dra-driver-cluster-worker
(...)
Spec:
  (...)
  Allocated Claims:
    d73cf7a9-dd2a-49f0-b5d5-ef2fd60b9079:
      Fake:
        Devices:
          Split:  2
          Uuid:   FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
          Split:  2
          Uuid:   FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
          Split:  2
          Uuid:   FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
          Split:  2
          Uuid:   FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
(...)
```

Next, looking at the `Prepared Devices` section of the `NodeAllocationState` object on the worker node will show which Fakes have been allocated and associated with parent device:

```console
$ kubectl describe -n fake-system nas/fake-dra-driver-cluster-worker
(...)
Spec:
  (...)
  Prepared Devices:
    d73cf7a9-dd2a-49f0-b5d5-ef2fd60b9079:
      Fake:
        Devices:
          Parent UUID:  FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
          Uuid:         FAKE-c3d6644d-29b2-c7b6-5393-95af636f14f8
          Parent UUID:  FAKE-b944f3e9-c628-b1dc-edb1-262aaee87362
          Uuid:         FAKE-b60e3cc1-54b4-1093-36b6-b8b0d59938c0
          Parent UUID:  FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
          Uuid:         FAKE-612391cf-f75b-bd17-fb90-92b63e26f37e
          Parent UUID:  FAKE-5bb9fd80-12c1-17f8-de47-7a3e76d565dc
          Uuid:         FAKE-5151740b-00c1-88f9-0d1b-018f6d8b3986
          Parent UUID:  FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
          Uuid:         FAKE-bcca8f6c-bacb-4aa3-dcfd-4398db24f464
          Parent UUID:  FAKE-0fc445e2-800c-177d-95e6-7fdad6b2427a
          Uuid:         FAKE-83fde35b-4eca-d530-17b0-390d716f4fdd
          Parent UUID:  FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
          Uuid:         FAKE-d54e4b82-88d3-5d4b-a30b-b692b3805590
          Parent UUID:  FAKE-4d6cc6e5-e22d-3807-f026-783d55841ea5
          Uuid:         FAKE-8e0f322e-bab9-0b81-64f1-eaf4736e2b9d
(...)
```

Once you have verified everything is running correctly, delete an example app:

```sh
kubectl delete --wait=false --filename=fake-test5.yaml
```

Finally, you can run the following to cleanup your environment and delete the `kind` cluster started previously:

```sh
./delete-cluster.sh
```

## References

For more information on the DRA Kubernetes feature and developing custom resource drivers, see the following resources:

* [Dynamic Resource Allocation in Kubernetes](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
* [kubernetes-sigs/dra-example-driver](https://github.com/kubernetes-sigs/dra-example-driver)
