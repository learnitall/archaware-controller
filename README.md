# archaware-controller

![Supported platforms are linux/amd64 and linux/arm](https://img.shields.io/badge/platform-linux%2Famd64%2Clinux%2Farm-informational)
![This project falls under the MIT License](https://img.shields.io/badge/license-MIT-informational)

*Architecture-aware placement of pods for Kubernetes*

This project was created in response to [kubernetes/kubernetes/issues/105321](https://github.com/kubernetes/kubernetes/issues/105321).

## What does it do?

archaware-controller is a Kubernetes [controller](https://kubernetes.io/docs/concepts/architecture/controller/) written purely in Go that simply does the following:

1. Adds `NoSchedule` taints to each of your nodes based on their architecture.
2. Adds tolerations to each of your Pods with their minimum set of supported architectures.

That's it.

## How does it do?

### Nodes

Kubernetes does the leg work here, as each node's architecture is available under its `status.nodeInfo`. All the controller does is find this value and use it to create a taint.

For instance, to view the architecture of your own cluster, copy and paste this large command:

`kubectl get nodes -o go-template='{{range .items}}{{index .metadata.labels "kubernetes.io/hostname"}} {{.status.nodeInfo.architecture}}{{printf "\n"}}{{end}}'`

### Pods

In order to add the correct toleration onto each pod, we have to deal with the fact that a pod can have more than one container, each running different images. What the controller does is find the intersection between the set of architectures of each image within a pod, using said intersection as the list of tolerable architectures.

For instance, if I have a pod with two containers, one which is single-platform on `amd64` and one which is multi-platform for `amd64`, `arm` and `ppc64le`, the pod will only be given the `amd64` toleration.

Under-the-hood, daemon-less [containerd](https://github.com/containerd/containerd) is used to inspect the manifest (or manifest list, or index, depending on your flavor of choice and if your image is multi-platform) of each image. This doesn't require that the image is pulled from its registry, meaning the controller has no large storage or network bandwidth requirements.

## Where does it do?

The archaware-controller is available as an image on [Docker Hub](https://hub.docker.com/repository/docker/learnitall/archaware-controller). It can also be installed via [archaware-controller.yaml](./archaware-controller.yaml), which creates:

* A service account for the controller
* A cluster role with list, watch, get and update permissions for nodes and pods
* A cluster role binding for the above cluster role onto the above service account
* A single-container deployment for the controller

Just `kubectl apply -f`.

## Caveats (does it do?)

This controller introduces a single point of failure in your cluster. If something happens and the controller can't function anymore, you have to either:

1. Remove the architecture taint from each node and delete all your pods so your Deployments, ReplicSets, DaemonSets, etc. can recreate them, as tolerations cannot be removed from pods. Running the architecture-controller manually as `go run . clean` will do this for you.
2. Manually add taints and tolerations onto new nodes and pods.

As stated above, since tolerations on pods cannot be removed, issues may arise if a container's image within a pod is changed to one that uses a different architecture. It is recommended that if this needs to happen, a new pod should be created.

## Contributing

The directory `testimg` can be used to generate example manifests and manifest lists (aka indices) for testing the controller's ability to grab architectures for an image. It requires that you have the following:

* A repository on DockerHub named `archaware-testimg`
* A respository on quay named `archaware-testimg`
* Docker installed
* Podman installed
* An authfile setup with quay.io credentials (`docker login` and `podman login` conflict with one another, see `podman login --authfile`)

Using [testimg/update.sh](./testimg/update.sh), you can build a test manifest and manifest list and push the result to both quay.io and DockerHub.

As soon as this is complete, tests for architecture resolution can be ran using:

`DOCKER_NS=<docker username> QUAY_NS=<quay username> go test .`

Say for instance that someone has a DockerHub account named `my_dockerhub_account` and a quay.io account named `my_quay_account`. They would execute the following from the project root:

```
docker login
podman login --authfile ./auth.json
cd testimg
./update.sh my_dockerhub_account my_quay_account ../auth.json
cd ..
DOCKER_NS=my_dockerhub_account QUAY_NS=my_quay_account go test .
```

The script [hak/build.sh](./hak/build.sh) can be used to build the archaware-controller image and push it to DockerHub. It requires that your docker CLI has buildx available:

```
docker login
./hak/build.sh my_dockerhub_account <tag>
```

## Next Steps

* Explore use of [RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/).
* Add support for a configuration file for more opinionated deployments.
* Create option for 'bootstrapping' the cluster before execution, by applying tolerations onto pods before taints on nodes are applied.
