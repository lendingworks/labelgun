[![](https://images.microbadger.com/badges/image/geota/labelgun.svg)](https://microbadger.com/images/geota/labelgun "Get your own image badge on microbadger.com")
[![](https://images.microbadger.com/badges/version/geota/labelgun.svg)](https://microbadger.com/images/geota/labelgun "Get your own version badge on microbadger.com")
[![Build Status](https://travis-ci.org/geota/labelgun.svg?branch=master)](https://travis-ci.org/geota/labelgun)

# labelgun

**Insert AWS EC2 Tags as Kubernetes Node Labels.**

Added a new feature on top of [DailyHotel/labelgun](https://github.com/DailyHotel/labelgun):

1) Support filtering tags by prefix to only label a subset of tags that may be on your EC2 instances.
2) Support adding taints to nodes.

This is the improved version of [Vungle/labelgun](https://github.com/Vungle/labelgun) in several aspects:

* [DaemonSet](https://github.com/kubernetes/kubernetes/blob/master/docs/design/daemon.md) is not required. Just launch a single pod and save the rest of your computational resources.
* Kubernetes version v1.5.x is supported
* Fine-grained logging
* Private base image Vungle/kubectl is removed
* Better developer support using `Makefile` and `glide.yaml`

## Supported:

* ec2tags
* ~~availability zone~~ and ~~instance type~~ are not supported any more since [Kubernetes itself provides the both](https://kubernetes.io/docs/admin/multiple-zones/).

## Configure

Edit the `labelgun.yml` with appropriate Environment Variable values for:

- [`LABELGUN_ERR_THRESHOLD`](https://godoc.org/github.com/golang/glog)  logging threshold
- `LABELGUN_INTERVAL` seconds to poll for new tags to apply labels and taints for.
- `LABELGUN_LABEL_TAG_PREFIX` prefix to filter tags - only adds labels to nodes for tags that match this prefix - set to `*` wildcard to label all tags (regex not supported).
- `LABELGUN_NO_SCHEDULE_TAG_PREFIX` prefix to filter tags - only adds NO_SCHEDULE taints to nodes for tags that match this prefix - set to `*` wildcard to add taints all tags (regex not supported).
- `LABELGUN_PREFER_NO_SCHEDULE_TAG_PREFIX` prefix to filter tags - only adds PREFER_NO_SCHEDULE taints to node for tags that match this prefix - set to `*` wildcard to add taints all tags (regex not supported).
- `LABELGUN_NO_EXECUTE_TAG_PREFIX` prefix to filter tags - only adds NO_EXECUT taints to node for tags that match this prefix - set to `*` wildcard to add taints all tags (regex not supported).

## RBAC
- Needs permissions to add labels and taints to nodes.
- As well as, list all nodes.

## Launch the Deployment

```yaml
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: labelgun
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: labelgun
    spec:
      containers:
        -
          env:
            -
              name: AWS_REGION
              value: us-west-2
            -
              name: LABELGUN_ERR_THRESHOLD
              value: INFO
            -
              name: LABELGUN_INTERVAL
              value: "360"
            -
              name: LABELGUN_LABEL_TAG_PREFIX
              value: rmn.io/
            -
              name: LABELGUN_NO_SCHEDULE_TAG_PREFIX
              value: rmn.io/no-schedule
            -
              name: LABELGUN_PREFER_NO_SCHEDULE_TAG_PREFIX
              value: rmn.io/prefer-no-schedule
            -
              name: LABELGUN_NO_EXECUTE_TAG_PREFIX
              value: rmn.io/no-execute
          image: "geota/labelgun:latest"
          imagePullPolicy: Always
          name: labelgun

```

`kubectl create -f labelgun.yml`

## Develop

``` bash
go get github.com/geota/labelgun

glide install --strip-vendor --strip-vcs

make
```
